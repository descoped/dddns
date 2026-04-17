package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/verify"
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

// runVerify performs DNS verification by delegating to verify.Run and
// formatting the resulting Report for stdout.
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := verify.Run(ctx, cfg)
	if err != nil {
		return err
	}

	fmt.Println("=== DNS Verification ===")
	fmt.Println()

	// 1. Public IP.
	fmt.Printf("Your public IP:     %s\n", report.PublicIP)

	// 2. Route53.
	if report.Route53Error != nil {
		fmt.Printf("Route53 record:     NOT FOUND (%v)\n", report.Route53Error)
	} else {
		fmt.Printf("Route53 record:     %s", report.Route53IP)
		if report.Route53IP == report.PublicIP {
			fmt.Printf(" ✓\n")
		} else {
			fmt.Printf(" ✗ (mismatch)\n")
		}
	}

	// 3. Stdlib DNS resolver.
	fmt.Printf("Public DNS lookup:  ")
	switch {
	case report.StdlibError != nil:
		fmt.Printf("FAILED (%v)\n", report.StdlibError)
	case report.StdlibIP == "":
		fmt.Printf("NO A RECORD\n")
	default:
		fmt.Printf("%s", report.StdlibIP)
		if report.StdlibIP == report.PublicIP {
			fmt.Printf(" ✓\n")
		} else {
			fmt.Printf(" ✗ (mismatch)\n")
		}
	}

	// 4. Named resolvers.
	fmt.Println()
	fmt.Println("DNS Server Checks:")
	for _, r := range report.Resolvers {
		fmt.Printf("  %s: ", r.Name)
		switch {
		case r.Error != nil:
			fmt.Printf("FAILED\n")
		case r.IP == "":
			fmt.Printf("NO RECORD\n")
		default:
			fmt.Printf("%s", r.IP)
			if r.IP == report.PublicIP {
				fmt.Printf(" ✓\n")
			} else {
				fmt.Printf(" ✗\n")
			}
		}
	}

	// Summary.
	fmt.Println()
	fmt.Println("=== Summary ===")
	switch {
	case report.Route53Error != nil || report.Route53IP == "":
		fmt.Println("⚠ No Route53 record found - run 'dddns update' to create it")
	case report.Route53IP == report.PublicIP:
		fmt.Println("✓ Route53 record is up to date")
	default:
		fmt.Printf("✗ Route53 record (%s) doesn't match current IP (%s)\n", report.Route53IP, report.PublicIP)
		fmt.Println("  Run 'dddns update' to fix this")
	}

	fmt.Printf("\nNote: DNS changes can take up to %d seconds to propagate globally.\n", cfg.TTL)

	return nil
}
