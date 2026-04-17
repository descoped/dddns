package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"strings"
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

var (
	serveTestHostname string
	serveTestIP       string
)

var serveTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a local Basic-Auth'd request to the serve-mode listener",
	Long: `Craft a dyndns-style GET to 127.0.0.1 on the configured bind port,
using the shared secret from the config. Prints the HTTP status and
response body. Exits 0 on "good" / "nochg", non-zero on any other
dyndns code or network failure.

This is the SSH debug path — run it from a shell on the router to
confirm the listener is reachable, the credential matches, and the
handler wiring produces an expected response.`,
	RunE: runServeTest,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.AddCommand(serveStatusCmd)
	serveCmd.AddCommand(serveTestCmd)

	serveTestCmd.Flags().StringVar(&serveTestHostname, "hostname", "", "Override hostname (default: cfg.Hostname)")
	serveTestCmd.Flags().StringVar(&serveTestIP, "ip", "1.2.3.4", "myip query param (handler ignores for the actual UPSERT — this is just for the wire-level test)")
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

func runServeTest(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Server == nil {
		return fmt.Errorf("serve mode not configured (no server block in config)")
	}

	hostname := serveTestHostname
	if hostname == "" {
		hostname = cfg.Hostname
	}

	return performServeTest(
		loopbackURL(cfg.Server.Bind),
		hostname,
		cfg.Server.SharedSecret,
		serveTestIP,
		cmd.OutOrStdout(),
	)
}

// performServeTest is the side-effect-free core of runServeTest. It is
// called with an explicit base URL so tests can point at an
// httptest.NewServer rather than a real loopback listener.
func performServeTest(baseURL, hostname, secret, myip string, out io.Writer) error {
	u := baseURL + "/nic/update?hostname=" + url.QueryEscape(hostname) + "&myip=" + url.QueryEscape(myip)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth("dddns", secret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	fmt.Fprintf(out, "HTTP %d\n", resp.StatusCode)
	fmt.Fprintf(out, "Body: %s\n", strings.TrimSpace(string(body)))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	fields := strings.Fields(strings.TrimSpace(string(body)))
	if len(fields) == 0 {
		return fmt.Errorf("server returned empty body")
	}
	switch fields[0] {
	case "good", "nochg":
		return nil
	default:
		return fmt.Errorf("dyndns code %q", fields[0])
	}
}

// loopbackURL builds a URL pointing at localhost from a bind address
// like "0.0.0.0:53353" or "127.0.0.1:53353".
func loopbackURL(bind string) string {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		return "http://" + bind
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
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
