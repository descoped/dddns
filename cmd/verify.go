package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
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

	formatVerifyReport(os.Stdout, report, cfg.TTL)
	return nil
}

// formatVerifyReport renders report to w in the human-readable shape
// the operator reads to decide whether the DNS setup is healthy.
// Extracted from runVerify so tests can assert on the formatter
// without exercising the real Route53/DNS network stack.
func formatVerifyReport(w io.Writer, report *verify.Report, ttl int64) {
	fmt.Fprintln(w, "=== DNS Verification ===")
	fmt.Fprintln(w)

	// 1. Public IP.
	fmt.Fprintf(w, "Your public IP:     %s\n", report.PublicIP)

	// 2. Route53.
	if report.Route53Error != nil {
		fmt.Fprintf(w, "Route53 record:     NOT FOUND (%v)\n", report.Route53Error)
	} else {
		fmt.Fprintf(w, "Route53 record:     %s", report.Route53IP)
		if report.Route53IP == report.PublicIP {
			fmt.Fprintf(w, " ✓\n")
		} else {
			fmt.Fprintf(w, " ✗ (mismatch)\n")
		}
	}

	// 3. Stdlib DNS resolver.
	fmt.Fprintf(w, "Public DNS lookup:  ")
	switch {
	case report.StdlibError != nil:
		fmt.Fprintf(w, "FAILED (%v)\n", report.StdlibError)
	case report.StdlibIP == "":
		fmt.Fprintf(w, "NO A RECORD\n")
	default:
		fmt.Fprintf(w, "%s", report.StdlibIP)
		if report.StdlibIP == report.PublicIP {
			fmt.Fprintf(w, " ✓\n")
		} else {
			fmt.Fprintf(w, " ✗ (mismatch)\n")
		}
	}

	// 4. Named resolvers.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "DNS Server Checks:")
	for _, r := range report.Resolvers {
		fmt.Fprintf(w, "  %s: ", r.Name)
		switch {
		case r.Error != nil:
			fmt.Fprintf(w, "FAILED\n")
		case r.IP == "":
			fmt.Fprintf(w, "NO RECORD\n")
		default:
			fmt.Fprintf(w, "%s", r.IP)
			if r.IP == report.PublicIP {
				fmt.Fprintf(w, " ✓\n")
			} else {
				fmt.Fprintf(w, " ✗\n")
			}
		}
	}

	// Summary.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Summary ===")
	switch {
	case report.Route53Error != nil || report.Route53IP == "":
		fmt.Fprintln(w, "⚠ No Route53 record found - run 'dddns update' to create it")
	case report.Route53IP == report.PublicIP:
		fmt.Fprintln(w, "✓ Route53 record is up to date")
	default:
		fmt.Fprintf(w, "✗ Route53 record (%s) doesn't match current IP (%s)\n", report.Route53IP, report.PublicIP)
		fmt.Fprintln(w, "  Run 'dddns update' to fix this")
	}

	fmt.Fprintf(w, "\nNote: DNS changes can take up to %d seconds to propagate globally.\n", ttl)
}
