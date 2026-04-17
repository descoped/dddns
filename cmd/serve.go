package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the serve-mode HTTP listener (event-driven via UniFi inadyn)",
	Long: `Start the HTTP listener that accepts dyndns updates from UniFi's
inadyn. Binds to cfg.Server.Bind (default 127.0.0.1:53353) and pushes
the router's authoritative WAN IP to Route53 on each valid request.

This is the alternative to cron-based updates. The two modes are
mutually exclusive — pick one at install time via
  dddns config set-mode {cron|serve}`,
	RunE: runServe,
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the last-request summary from the serve-mode status file",
	Long: `Print a human-readable summary of the last HTTP request the
serve-mode listener handled: when it arrived, where from, the auth
outcome, the resulting action, and any error. Reads
<data-dir>/serve-status.json written by the server on every request.`,
	RunE: runServeStatus,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.AddCommand(serveStatusCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

func runServeStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	path := server.StatusPath(cfg)
	snap, err := server.ReadStatus(path)
	if err != nil {
		return fmt.Errorf("%w\n(is `dddns serve` running? no status is recorded until the server handles its first request)", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Status file:    %s\n", path)
	fmt.Fprintf(out, "Last request:   %s\n", snap.LastRequestAt.Format(time.RFC3339))
	if snap.LastRemoteAddr != "" {
		fmt.Fprintf(out, "Remote:         %s\n", snap.LastRemoteAddr)
	}
	if snap.LastAuthOutcome != "" {
		fmt.Fprintf(out, "Auth outcome:   %s\n", snap.LastAuthOutcome)
	}
	if snap.LastAction != "" {
		fmt.Fprintf(out, "Action:         %s\n", snap.LastAction)
	}
	if snap.LastError != "" {
		fmt.Fprintf(out, "Error:          %s\n", snap.LastError)
	}
	return nil
}
