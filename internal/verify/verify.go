// Package verify produces a DNS-propagation snapshot for the `dddns
// verify` command. It isolates the network calls from the CLI layer so
// cmd/verify.go can stay focused on formatting.
package verify

import (
	"context"
	"net"
	"time"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/dns"
)

// ResolverResult captures the outcome of a single named DNS server lookup.
type ResolverResult struct {
	Name   string // human-readable label (e.g. "Google")
	Server string // resolver address in host:port form
	IP     string // resolved IPv4 address (empty on error or NODATA)
	Error  error  // non-nil when the resolver failed
}

// Report is the snapshot returned by Run.
type Report struct {
	PublicIP     string
	Route53IP    string
	Route53Error error
	StdlibIP     string
	StdlibError  error
	Resolvers    []ResolverResult
}

// namedResolvers is the canonical set of public DNS servers the verify
// flow consults. Order matters only for output stability.
var namedResolvers = []struct {
	Name    string
	Address string
}{
	{"Google", "8.8.8.8:53"},
	{"Cloudflare", "1.1.1.1:53"},
	{"Quad9", "9.9.9.9:53"},
}

// Run executes the full verify flow. It is safe to call with a cancelled
// context — each sub-step honours ctx. A non-nil error is returned only
// when the initial public-IP lookup fails; per-step failures (Route53,
// stdlib, named resolvers) are folded into the Report so the caller can
// display partial results.
func Run(ctx context.Context, cfg *config.Config) (*Report, error) {
	publicIP, err := myip.GetPublicIP(ctx)
	if err != nil {
		return nil, err
	}

	rep := &Report{PublicIP: publicIP}

	// Route53.
	r53, err := dns.NewFromConfig(ctx, cfg)
	if err != nil {
		rep.Route53Error = err
	} else {
		r53Ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		ip, err := r53.GetCurrentIP(r53Ctx)
		cancel()
		if err != nil {
			rep.Route53Error = err
		} else {
			rep.Route53IP = ip
		}
	}

	// Stdlib lookup.
	stdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	ips, lookupErr := net.DefaultResolver.LookupIPAddr(stdCtx, cfg.Hostname)
	cancel()
	if lookupErr != nil {
		rep.StdlibError = lookupErr
	} else {
		for _, ip := range ips {
			if v4 := ip.IP.To4(); v4 != nil {
				rep.StdlibIP = v4.String()
				break
			}
		}
	}

	// Named resolvers.
	rep.Resolvers = make([]ResolverResult, 0, len(namedResolvers))
	for _, nr := range namedResolvers {
		res := ResolverResult{Name: nr.Name, Server: nr.Address}
		ip, err := queryResolver(ctx, cfg.Hostname, nr.Address)
		if err != nil {
			res.Error = err
		} else {
			res.IP = ip
		}
		rep.Resolvers = append(rep.Resolvers, res)
	}

	return rep, nil
}

// queryResolver issues an A lookup for hostname against a specific DNS
// server (host:port form). It honours ctx and additionally caps its own
// dial + lookup wall-clock to 2 seconds for defensive isolation.
func queryResolver(ctx context.Context, hostname, server string) (string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, network, server)
		},
	}
	qCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ips, err := r.LookupIPAddr(qCtx, hostname)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", nil
	}
	return ips[0].IP.String(), nil
}
