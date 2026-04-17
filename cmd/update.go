package cmd

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/updater"
	"github.com/spf13/cobra"
)

var (
	forceUpdate bool
	dryRun      bool
	customIP    string
	quiet       bool
)

// updateTimeout is the maximum wall-clock time an `update` run may take.
// Bounds Route53 hangs so cron doesn't accumulate stuck processes.
const updateTimeout = 30 * time.Second

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Route53 DNS record with current public IP",
	Long: `Check current public IP address and update Route53 DNS A record if changed.
This command is designed to be run from cron every 30 minutes.`,
	RunE: runUpdate,
}

// init registers the update command and its flags.
func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().BoolVarP(&forceUpdate, "force", "f", false, "Force update even if IP hasn't changed")
	updateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without making changes")
	updateCmd.Flags().StringVar(&customIP, "ip", "", "Use specific IP address instead of auto-detecting")
	updateCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output (for cron)")
}

// runUpdate wires the cobra command to the updater package. It builds a
// context that cancels on SIGINT/SIGTERM or after updateTimeout, validates
// any --ip override, and delegates the rest to updater.Update.
func runUpdate(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Cancel on SIGINT/SIGTERM; bound total runtime.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	if !quiet {
		log.Printf("[%s] Checking for IP changes...", time.Now().Format("2006-01-02 15:04:05"))
	}

	opts := updater.Options{
		Force:  forceUpdate,
		DryRun: dryRun,
		Quiet:  quiet,
	}
	if customIP != "" {
		if err := myip.ValidatePublicIP(customIP); err != nil {
			return fmt.Errorf("invalid --ip value: %w", err)
		}
		opts.OverrideIP = customIP
	}

	_, err = updater.Update(ctx, cfg, opts)
	return err
}
