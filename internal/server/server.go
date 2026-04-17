package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/descoped/dddns/internal/config"
)

// Server wraps the HTTP listener that backs `dddns serve`. Dependencies
// are constructed in NewServer; Run blocks until the provided context
// is cancelled, then shuts down gracefully.
type Server struct {
	http   *http.Server
	binder func() error // set in Run; allows tests to swap in an httptest listener
}

// NewServer wires the handler chain from a validated Config. Both
// Config.Validate and ServerConfig.Validate are called — fail-closed
// startup per §3 L6.
func NewServer(cfg *config.Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config invalid: %w", err)
	}
	if cfg.Server == nil {
		return nil, fmt.Errorf("serve mode requires a server block in config")
	}
	if err := cfg.Server.Validate(); err != nil {
		return nil, fmt.Errorf("server config invalid: %w", err)
	}

	auth := NewAuthenticator(cfg.Server.SharedSecret)
	audit := NewAuditLog(AuditPath(cfg))
	status := NewStatusWriter(StatusPath(cfg))
	handler := NewHandler(cfg, auth, audit, status)

	mux := http.NewServeMux()
	mux.Handle("/nic/update", handler)

	httpSrv := &http.Server{
		Addr:              cfg.Server.Bind,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      35 * time.Second, // must exceed handlerTimeout (30s)
		IdleTimeout:       30 * time.Second,
	}
	return &Server{
		http:   httpSrv,
		binder: func() error { return httpSrv.ListenAndServe() },
	}, nil
}

// Run starts the listener and blocks until ctx is cancelled. On
// cancellation it performs an http.Server.Shutdown with a 5-second
// deadline, draining any in-flight request.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.binder()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Handler returns the underlying http.Handler. Exposed for tests that
// want to use httptest.NewServer instead of binding a real port.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// AuditPath returns the audit-log path — user-configured or derived
// from the data directory. Exported so cmd/ callers (e.g.
// `dddns serve status`) don't need to duplicate the path logic.
func AuditPath(cfg *config.Config) string {
	if cfg.Server != nil && cfg.Server.AuditLog != "" {
		return cfg.Server.AuditLog
	}
	return filepath.Join(filepath.Dir(cfg.IPCacheFile), "serve-audit.log")
}

// StatusPath returns the serve-status.json path — always in the same
// directory as the IP cache.
func StatusPath(cfg *config.Config) string {
	return filepath.Join(filepath.Dir(cfg.IPCacheFile), "serve-status.json")
}
