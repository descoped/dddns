package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/dns"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	forceUpdate bool
	dryRun      bool
	customIP    string
	quiet       bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Route53 DNS record with current public IP",
	Long: `Check current public IP address and update Route53 DNS A record if changed.
This command is designed to be run from cron every 30 minutes.`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().BoolVarP(&forceUpdate, "force", "f", false, "Force update even if IP hasn't changed")
	updateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without making changes")
	updateCmd.Flags().StringVar(&customIP, "ip", "", "Use specific IP address instead of auto-detecting")
	updateCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output (for cron)")

	_ = viper.BindPFlag("force", updateCmd.Flags().Lookup("force"))
	_ = viper.BindPFlag("dry-run", updateCmd.Flags().Lookup("dry-run"))
	_ = viper.BindPFlag("quiet", updateCmd.Flags().Lookup("quiet"))
}

// logInfo logs only if not in quiet mode
func logInfo(format string, args ...interface{}) {
	if !quiet {
		log.Printf(format, args...)
	}
}

func runUpdate(_ *cobra.Command, _ []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Log start
	logInfo("[%s] Checking for IP changes...", time.Now().Format("2006-01-02 15:04:05"))

	// 1. Get current public IP (or use custom IP if provided)
	var currentIP string
	if customIP != "" {
		// Validate custom IP
		if net.ParseIP(customIP) == nil {
			return fmt.Errorf("invalid IP address: %s", customIP)
		}
		currentIP = customIP
		logInfo("Using custom IP: %s", currentIP)
	} else {
		detectedIP, err := myip.GetPublicIP()
		if err != nil {
			return fmt.Errorf("failed to get public IP: %w", err)
		}
		currentIP = strings.TrimSpace(detectedIP)
		logInfo("Current public IP: %s", currentIP)
	}

	// 2. Check cached IP
	cachedIP := readCachedIP(cfg.IPCacheFile)
	if cachedIP != "" && !quiet {
		logInfo("Last known IP: %s", cachedIP)
	}

	// 3. Check if update needed
	if !cfg.ForceUpdate && currentIP == cachedIP {
		if !quiet {
			logInfo("IP unchanged (%s), skipping update", currentIP)
		}
		return nil
	}

	// 4. Check if proxy (optional) - skip for custom IP
	if !cfg.SkipProxy && customIP == "" {
		isProxy, err := myip.IsProxyIP(&currentIP)
		if err != nil {
			logInfo("Warning: proxy check failed: %v", err)
		} else if isProxy {
			return fmt.Errorf("proxy/VPN detected for IP %s, skipping update", currentIP)
		}
	}

	// 5. Connect to Route53
	r53Client, err := dns.NewRoute53Client(cfg.AWSRegion, cfg.AWSAccessKey, cfg.AWSSecretKey, cfg.HostedZoneID, cfg.Hostname, cfg.TTL)
	if err != nil {
		return fmt.Errorf("failed to create Route53 client: %w", err)
	}

	// 6. Get current DNS record
	dnsIP, err := r53Client.GetCurrentIP()
	if err != nil {
		logInfo("Warning: could not get current DNS record: %v", err)
		// Continue anyway - the record might not exist yet
	} else {
		logInfo("Current DNS record: %s", dnsIP)

		// Check if DNS already has correct IP
		if currentIP == dnsIP && !cfg.ForceUpdate {
			logInfo("DNS already up to date with %s", currentIP)
			// Still update cache file
			if err := writeCachedIP(cfg.IPCacheFile, currentIP); err != nil {
				logInfo("Warning: failed to update cache file: %v", err)
			}
			return nil
		}
	}

	// 7. Update Route53 (or show what would be done)
	if cfg.DryRun {
		log.Printf("[DRY RUN] Would update %s to %s (TTL: %d)", cfg.Hostname, currentIP, cfg.TTL)
		if cachedIP != "" {
			log.Printf("[DRY RUN] Would update cache from %s to %s", cachedIP, currentIP)
		}
	} else {
		logInfo("Updating %s to %s...", cfg.Hostname, currentIP)
		if err := r53Client.UpdateIP(currentIP, cfg.DryRun); err != nil {
			return fmt.Errorf("failed to update Route53: %w", err)
		}
		// Always show successful updates, even in quiet mode
		log.Printf("Successfully updated %s to %s", cfg.Hostname, currentIP)

		// 8. Update cache file
		if err := writeCachedIP(cfg.IPCacheFile, currentIP); err != nil {
			logInfo("Warning: failed to update cache file: %v", err)
			// Don't fail the whole operation for this
		}
	}

	return nil
}

// readCachedIP reads the last known IP from cache file
func readCachedIP(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist or can't be read - that's okay
		return ""
	}

	// Parse YAML format: "last_known_ip: x.x.x.x"
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "last_known_ip:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "last_known_ip:"))
		}
	}

	// Fallback for old format (just IP)
	ip := strings.TrimSpace(string(data))
	if net.ParseIP(ip) != nil {
		return ip
	}

	return ""
}

// writeCachedIP writes the current IP to cache file with timestamp
func writeCachedIP(path string, ip string) error {
	// Ensure directory exists
	dir := path[:strings.LastIndex(path, "/")]
	if err := os.MkdirAll(dir, constants.CacheDirPerm); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create YAML format with timestamp
	content := fmt.Sprintf("last_known_ip: %s\nlast_updated: %s\n",
		ip,
		time.Now().Format(time.RFC3339))

	// Write to file
	if err := os.WriteFile(path, []byte(content), constants.CacheFilePerm); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}
