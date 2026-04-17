package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

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

func init() {
	rootCmd.AddCommand(serveCmd)
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
