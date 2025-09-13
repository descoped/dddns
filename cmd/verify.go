package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/dns"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify DNS record matches current IP",
	Long:  `Check if the DNS record is correctly pointing to your current public IP address.`,
	RunE:  runVerify,
}

// init registers the verify command.
func init() {
	rootCmd.AddCommand(verifyCmd)
}

// checkDNSServer queries a specific DNS server for the hostname and compares with expected IP.
// It prints the result with visual indicators for match/mismatch.
func checkDNSServer(hostname, server, expectedIP string) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Second * 2,
			}
			return d.DialContext(ctx, network, server)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ips, err := r.LookupIPAddr(ctx, hostname)
	if err != nil {
		fmt.Printf("FAILED\n")
	} else if len(ips) == 0 {
		fmt.Printf("NO RECORD\n")
	} else {
		ip := ips[0].IP.String()
		fmt.Printf("%s", ip)
		if ip == expectedIP {
			fmt.Printf(" ✓\n")
		} else {
			fmt.Printf(" ✗\n")
		}
	}
}

// runVerify performs DNS verification:
// 1. Gets current public IP
// 2. Queries Route53 for current DNS record
// 3. Tests resolution from multiple DNS servers
// 4. Reports propagation status
func runVerify(_ *cobra.Command, _ []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	fmt.Println("=== DNS Verification ===")
	fmt.Println()

	// 1. Get current public IP
	currentIP, err := myip.GetPublicIP()
	if err != nil {
		return fmt.Errorf("failed to get public IP: %w", err)
	}
	currentIP = strings.TrimSpace(currentIP)
	fmt.Printf("Your public IP:     %s\n", currentIP)

	// 2. Check Route53 record
	r53Client, err := dns.NewRoute53Client(cfg.AWSRegion, cfg.AWSAccessKey, cfg.AWSSecretKey, cfg.HostedZoneID, cfg.Hostname, cfg.TTL)
	if err != nil {
		return fmt.Errorf("failed to create Route53 client: %w", err)
	}

	route53IP, err := r53Client.GetCurrentIP()
	if err != nil {
		log.Printf("Route53 record:     NOT FOUND (%v)", err)
	} else {
		fmt.Printf("Route53 record:     %s", route53IP)
		if route53IP == currentIP {
			fmt.Printf(" ✓\n")
		} else {
			fmt.Printf(" ✗ (mismatch)\n")
		}
	}

	// 3. Check public DNS resolution
	fmt.Printf("Public DNS lookup:  ")
	ips, err := net.LookupIP(cfg.Hostname)
	if err != nil {
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		foundIP := ""
		for _, ip := range ips {
			if ip.To4() != nil { // IPv4 only
				foundIP = ip.String()
				break
			}
		}
		if foundIP == "" {
			fmt.Printf("NO A RECORD\n")
		} else {
			fmt.Printf("%s", foundIP)
			if foundIP == currentIP {
				fmt.Printf(" ✓\n")
			} else {
				fmt.Printf(" ✗ (mismatch)\n")
			}
		}
	}

	// 4. Check multiple DNS servers
	fmt.Println()
	fmt.Println("DNS Server Checks:")
	dnsServers := map[string]string{
		"Google":     "8.8.8.8:53",
		"Cloudflare": "1.1.1.1:53",
		"Quad9":      "9.9.9.9:53",
	}

	for name, server := range dnsServers {
		fmt.Printf("  %s: ", name)

		// Extract DNS check to avoid defer in loop
		checkDNSServer(cfg.Hostname, server, currentIP)
	}

	// Summary
	fmt.Println()
	fmt.Println("=== Summary ===")
	if route53IP == currentIP {
		fmt.Println("✓ Route53 record is up to date")
	} else if route53IP == "" {
		fmt.Println("⚠ No Route53 record found - run 'dddns update' to create it")
	} else {
		fmt.Printf("✗ Route53 record (%s) doesn't match current IP (%s)\n", route53IP, currentIP)
		fmt.Println("  Run 'dddns update' to fix this")
	}

	fmt.Printf("\nNote: DNS changes can take up to %d seconds to propagate globally.\n", cfg.TTL)

	return nil
}
