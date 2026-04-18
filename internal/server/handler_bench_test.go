package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/updater"
)

// BenchmarkHandler_HappyPath drives the wired serve-mode handler's
// ServeHTTP directly (no TCP socket) through a large batch of
// authenticated, allowed, hostname-match requests with all upstreams
// stubbed. The ReportAllocs output lets us track allocations-per-request
// across releases; a regression where a request starts retaining an
// extra slice / map / string shows up immediately.
//
// Bypassing the TCP layer is deliberate — driving this through
// httptest.NewServer saturates ephemeral ports on macOS around 30k
// requests, which is a client-side TCP artefact, not a handler
// property. ServeHTTP captures exactly the request-scoped allocation
// profile we care about.
//
// Run with:
//
//	go test -bench . -benchmem -memprofile=/tmp/h.out ./internal/server/
//	go tool pprof -alloc_space /tmp/h.out
func BenchmarkHandler_HappyPath(b *testing.B) {
	tmp := b.TempDir()
	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     testHostname,
		TTL:          300,
		IPCacheFile:  filepath.Join(tmp, "cache.txt"),
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:0",
			SharedSecret: testSecretV,
			AllowedCIDRs: []string{"127.0.0.0/8"},
			AuditLog:     filepath.Join(tmp, "audit.log"),
		},
	}
	srv, err := NewServer(cfg)
	if err != nil {
		b.Fatal(err)
	}
	mux := srv.http.Handler.(*http.ServeMux)
	h, _ := mux.Handler(httptest.NewRequest(http.MethodGet, "/nic/update", nil))
	hdl := h.(*Handler)
	hdl.wanIP = func(string) (net.IP, error) { return net.ParseIP(testPublicIP), nil }
	hdl.updateIP = func(_ context.Context, _ *config.Config, opts updater.Options) (*updater.Result, error) {
		return &updater.Result{Action: "updated", NewIP: opts.OverrideIP, Hostname: cfg.Hostname}, nil
	}

	// Sample heap at the start so we can compare at the end — a flat or
	// shrinking delta across a large N is strong evidence no request-
	// scoped state is being retained.
	var startStats, endStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&startStats)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet,
			"/nic/update?hostname="+testHostname+"&myip="+testPublicIP, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.SetBasicAuth("dddns", testSecretV)
		rec := httptest.NewRecorder()
		hdl.ServeHTTP(rec, req)
	}
	b.StopTimer()

	runtime.GC()
	runtime.ReadMemStats(&endStats)
	b.Logf("HeapInUse delta: %d bytes (start=%d, end=%d) across N=%d requests",
		int64(endStats.HeapInuse)-int64(startStats.HeapInuse),
		startStats.HeapInuse, endStats.HeapInuse, b.N)
}
