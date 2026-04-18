// Package updater implements the core DNS update flow shared by the cron
// path (cmd/update.go) and the serve handler (internal/server).
package updater

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/dns"
	"github.com/descoped/dddns/internal/profile"
	"github.com/descoped/dddns/internal/wanip"
)

// resolver bundles the IP-source lookup seams. Production code constructs
// a defaultResolver; tests use updateWithResolver to swap any of the three
// hooks with a deterministic stub.
type resolver struct {
	localIP  func(iface string) (string, error)
	remoteIP func(ctx context.Context) (string, error)
	profile  func() string
}

// defaultResolver returns the resolver wired to real OS/network/profile
// calls. Callers that need to avoid the network must build their own.
func defaultResolver() *resolver {
	return &resolver{
		localIP: func(iface string) (string, error) {
			ip, err := wanip.FromInterface(iface)
			if err != nil {
				return "", err
			}
			return ip.String(), nil
		},
		remoteIP: myip.GetPublicIP,
		profile: func() string {
			return profile.Detect().Name
		},
	}
}

// resolveIP picks between the local WAN interface and a remote lookup
// based on cfg.IPSource. Empty / "auto" defaults to local on the UDM
// profile, remote elsewhere. The returned description is human-readable
// ("local (auto → udm profile)", "remote (cfg.ip_source=remote)") and is
// only consumed by --verbose — production paths ignore it.
func (r *resolver) resolveIP(ctx context.Context, cfg *config.Config) (ip, description string, err error) {
	source := cfg.IPSource
	autoDecision := ""
	if source == "" || source == "auto" {
		if r.profile() == "udm" {
			source = "local"
			autoDecision = "auto → udm profile"
		} else {
			source = "remote"
			autoDecision = "auto → non-udm profile"
		}
	}
	switch source {
	case "local":
		iface := ""
		if cfg.Server != nil {
			iface = cfg.Server.WANInterface
		}
		ip, err = r.localIP(iface)
		description = fmt.Sprintf("local (iface=%q)", iface)
		if autoDecision != "" {
			description = fmt.Sprintf("local (%s, iface=%q)", autoDecision, iface)
		}
		return ip, description, err
	case "remote":
		ip, err = r.remoteIP(ctx)
		description = "remote (checkip.amazonaws.com)"
		if autoDecision != "" {
			description = fmt.Sprintf("remote (%s, checkip.amazonaws.com)", autoDecision)
		}
		return ip, description, err
	default:
		return "", "", fmt.Errorf("unknown ip_source %q", source)
	}
}

// DNSClient is the subset of internal/dns.Route53Client that the updater
// exercises. Declaring it here lets tests inject a mock without constructing
// a real AWS client. dns.Route53Client satisfies this interface.
type DNSClient interface {
	GetCurrentIP(ctx context.Context) (string, error)
	UpdateIP(ctx context.Context, newIP string) error
}

// Options controls a single update run.
type Options struct {
	Force      bool
	DryRun     bool
	Quiet      bool
	Verbose    bool   // emit per-step diagnostic output (source choice, interface, TTL)
	OverrideIP string // empty = resolve via myip.GetPublicIP (default cron behavior)

	// Client, if set, replaces the Route53 client the updater would otherwise
	// construct from cfg. Intended for tests and for the serve handler.
	Client DNSClient
}

// Result describes the outcome of Update.
type Result struct {
	Action   string // "updated" | "nochg-cache" | "nochg-dns" | "dry-run"
	OldIP    string
	NewIP    string
	Hostname string
}

// Update performs the full update flow: resolve IP → compare cache →
// compare DNS → upsert → update cache.
func Update(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	return updateWithResolver(ctx, cfg, opts, defaultResolver())
}

// updateWithResolver is the production entry point's core. It is exposed
// (within-package) so tests can inject a deterministic resolver.
func updateWithResolver(ctx context.Context, cfg *config.Config, opts Options, res *resolver) (*Result, error) {
	logInfo := func(format string, args ...interface{}) {
		if !opts.Quiet {
			log.Printf(format, args...)
		}
	}
	logVerbose := func(format string, args ...interface{}) {
		if opts.Verbose {
			log.Printf(format, args...)
		}
	}

	// 1. Resolve current IP (override, or dispatch on cfg.IPSource).
	currentIP := opts.OverrideIP
	if currentIP == "" {
		detected, source, err := res.resolveIP(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to get public IP: %w", err)
		}
		currentIP = detected
		logVerbose("IP source: %s", source)
		logInfo("Current public IP: %s", currentIP)
	} else {
		logInfo("Using custom IP: %s", currentIP)
	}

	// 2. Compare against cache.
	cachedIP := readCachedIP(cfg.IPCacheFile)
	if cachedIP != "" {
		logInfo("Last known IP: %s", cachedIP)
	}

	if !opts.Force && currentIP == cachedIP {
		logInfo("IP unchanged (%s), skipping update", currentIP)
		return &Result{
			Action:   "nochg-cache",
			OldIP:    cachedIP,
			NewIP:    currentIP,
			Hostname: cfg.Hostname,
		}, nil
	}

	// 3. Create or use injected DNS client.
	client := opts.Client
	if client == nil {
		r53, err := dns.NewFromConfig(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Route53 client: %w", err)
		}
		client = r53
	}

	// 4. Compare against DNS.
	var dnsIP string
	if ip, err := client.GetCurrentIP(ctx); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		logInfo("Warning: could not get current DNS record: %v", err)
	} else {
		dnsIP = ip
		logInfo("Current DNS record: %s", dnsIP)
		if currentIP == dnsIP && !opts.Force {
			logInfo("DNS already up to date with %s", currentIP)
			if werr := writeCachedIP(cfg.IPCacheFile, currentIP); werr != nil {
				logInfo("Warning: failed to update cache file: %v", werr)
			}
			return &Result{
				Action:   "nochg-dns",
				OldIP:    dnsIP,
				NewIP:    currentIP,
				Hostname: cfg.Hostname,
			}, nil
		}
	}

	// 5. Dry-run short-circuit.
	if opts.DryRun {
		log.Printf("[DRY RUN] Would update %s to %s (TTL: %d)", cfg.Hostname, currentIP, cfg.TTL)
		if cachedIP != "" {
			log.Printf("[DRY RUN] Would update cache from %s to %s", cachedIP, currentIP)
		}
		return &Result{
			Action:   "dry-run",
			OldIP:    dnsIP,
			NewIP:    currentIP,
			Hostname: cfg.Hostname,
		}, nil
	}

	// 6. UPSERT.
	logInfo("Updating %s to %s...", cfg.Hostname, currentIP)
	if err := client.UpdateIP(ctx, currentIP); err != nil {
		return nil, fmt.Errorf("failed to update Route53: %w", err)
	}
	log.Printf("Successfully updated %s to %s", cfg.Hostname, currentIP)

	// 7. Refresh cache.
	if err := writeCachedIP(cfg.IPCacheFile, currentIP); err != nil {
		logInfo("Warning: failed to update cache file: %v", err)
	}

	return &Result{
		Action:   "updated",
		OldIP:    dnsIP,
		NewIP:    currentIP,
		Hostname: cfg.Hostname,
	}, nil
}

// readCachedIP reads the last known IP from cache file.
// Supports both the current YAML format and the legacy bare-IP format.
func readCachedIP(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// YAML format: "last_known_ip: x.x.x.x"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "last_known_ip:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "last_known_ip:"))
		}
	}

	// Legacy format (bare IP).
	ip := strings.TrimSpace(string(data))
	if net.ParseIP(ip) != nil {
		return ip
	}

	return ""
}

// writeCachedIP writes the current IP to the cache file with a timestamp.
func writeCachedIP(path, ip string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.CacheDirPerm); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	content := fmt.Sprintf("last_known_ip: %s\nlast_updated: %s\n", ip, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(content), constants.CacheFilePerm); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	return nil
}
