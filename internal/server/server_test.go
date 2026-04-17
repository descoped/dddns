package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/updater"
)

func validConfig(t *testing.T) *config.Config {
	t.Helper()
	tmp := t.TempDir()
	return &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     testHostname,
		TTL:          300,
		IPCacheFile:  filepath.Join(tmp, "cache.txt"),
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:0", // let the OS pick a port
			SharedSecret: testSecretV,
			AllowedCIDRs: []string{"127.0.0.0/8"},
			AuditLog:     filepath.Join(tmp, "audit.log"),
		},
	}
}

func TestNewServer_ValidConfig(t *testing.T) {
	srv, err := NewServer(validConfig(t))
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if srv.http.Addr != "127.0.0.1:0" {
		t.Errorf("Addr = %q", srv.http.Addr)
	}
}

func TestNewServer_InvalidConfigTopLevel(t *testing.T) {
	cfg := validConfig(t)
	cfg.Hostname = "" // Config.Validate rejects
	if _, err := NewServer(cfg); err == nil {
		t.Error("expected error for invalid top-level config")
	}
}

func TestNewServer_MissingServerBlock(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server = nil
	if _, err := NewServer(cfg); err == nil {
		t.Error("expected error when server block is absent")
	}
}

func TestNewServer_InvalidServerBlock(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.AllowedCIDRs = nil // ServerConfig.Validate rejects empty list
	if _, err := NewServer(cfg); err == nil {
		t.Error("expected error when ServerConfig.Validate fails")
	}
}

// TestServer_Integration_EndToEnd spins up the wired handler via
// httptest (bypasses ListenAndServe) and verifies dyndns responses
// come back over a real HTTP socket. Route53 and wanip are stubbed so
// no network / AWS is touched.
func TestServer_Integration_EndToEnd(t *testing.T) {
	cfg := validConfig(t)
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Reach into the wired handler to stub its upstreams. The concrete
	// *Handler sits on the mux at /nic/update.
	mux := srv.http.Handler.(*http.ServeMux)
	handler, _ := mux.Handler(httptest.NewRequest(http.MethodGet, "/nic/update", nil))
	h, ok := handler.(*Handler)
	if !ok {
		t.Fatalf("handler type = %T, want *Handler", handler)
	}
	h.wanIP = func(string) (net.IP, error) { return net.ParseIP("81.191.174.72"), nil }
	h.updateIP = func(_ context.Context, _ *config.Config, opts updater.Options) (*updater.Result, error) {
		return &updater.Result{Action: "updated", NewIP: opts.OverrideIP, Hostname: cfg.Hostname}, nil
	}

	// Expose via httptest.
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/nic/update?hostname="+testHostname+"&myip=81.191.174.72", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("dddns", testSecretV)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if got := strings.TrimSpace(string(body)); got != "good 81.191.174.72" {
		t.Errorf("body = %q", got)
	}
}

// TestServer_Run_GracefulShutdown verifies Run listens, serves requests,
// and exits cleanly when ctx is cancelled.
func TestServer_Run_GracefulShutdown(t *testing.T) {
	cfg := validConfig(t)
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Bind to a real ephemeral port instead of calling ListenAndServe,
	// so we know the actual address before Run starts and can hit it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	srv.listenAndServe = func() error { return srv.http.Serve(ln) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Give Serve a moment to start accepting.
	time.Sleep(50 * time.Millisecond)

	// Unauthenticated request — should come back with 200 + badauth body.
	resp, err := http.Get("http://" + addr + "/nic/update")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil on graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of ctx cancel")
	}
}
