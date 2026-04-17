package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
)

// testPublicIP is the single source of truth for the placeholder public
// IPv4 used across updater test fixtures. RFC 5737 TEST-NET-3.
const testPublicIP = "203.0.113.42"

// --- cache helpers (moved from cmd/update_test.go) ---

func TestReadCachedIP(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "cached-ip.txt")

	if ip := readCachedIP(cacheFile); ip != "" {
		t.Errorf("Expected empty string for non-existent file, got %q", ip)
	}

	testIP := "192.168.1.1"
	if err := os.WriteFile(cacheFile, []byte(testIP), 0600); err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}

	if ip := readCachedIP(cacheFile); ip != testIP {
		t.Errorf("Expected %q, got %q", testIP, ip)
	}
}

func TestWriteCachedIP(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "cached-ip.txt")

	if err := writeCachedIP(cacheFile, "10.0.0.1"); err != nil {
		t.Fatalf("Failed to write cached IP: %v", err)
	}

	content, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("Failed to read cache file: %v", err)
	}
	if !strings.Contains(string(content), "10.0.0.1") {
		t.Errorf("Expected cache to contain %q, got %q", "10.0.0.1", string(content))
	}

	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatalf("Failed to stat cache file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("Expected permissions 0600, got %04o", mode)
	}
}

func TestWriteCachedIP_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "nested", "deeper", "cache.txt")

	if err := writeCachedIP(cacheFile, "1.2.3.4"); err != nil {
		t.Fatalf("writeCachedIP failed on nested path: %v", err)
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("cache file not created at %s: %v", cacheFile, err)
	}
}

func TestWriteCachedIP_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	if err := writeCachedIP("cache.txt", "1.2.3.4"); err != nil {
		t.Fatalf("writeCachedIP failed on bare filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "cache.txt")); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

// --- Update() tests with injected DNS client ---

// fakeDNSClient is an injectable DNSClient that lets tests drive GetCurrentIP
// and UpdateIP responses and capture what the updater called with.
type fakeDNSClient struct {
	getIP        string
	getErr       error
	updateErr    error
	updateCalled bool
	updateIP     string
}

func (f *fakeDNSClient) GetCurrentIP(_ context.Context) (string, error) {
	return f.getIP, f.getErr
}

func (f *fakeDNSClient) UpdateIP(_ context.Context, newIP string) error {
	f.updateCalled = true
	f.updateIP = newIP
	return f.updateErr
}

// blockingDNSClient blocks both methods until ctx is cancelled.
type blockingDNSClient struct{}

func (blockingDNSClient) GetCurrentIP(ctx context.Context) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (blockingDNSClient) UpdateIP(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func baseConfig(tmpDir string) *config.Config {
	return &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "test",
		AWSSecretKey: "test",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		IPCacheFile:  filepath.Join(tmpDir, "cache.txt"),
	}
}

// TestUpdate_OverrideIP verifies that OverrideIP bypasses the public-IP
// lookup and gets pushed to Route53 when the DNS record doesn't match.
func TestUpdate_OverrideIP(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := baseConfig(tmpDir)
	fake := &fakeDNSClient{getIP: "1.1.1.1"} // existing record differs

	result, err := Update(context.Background(), cfg, Options{
		OverrideIP: "5.6.7.8",
		Client:     fake,
		Quiet:      true,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result.Action != "updated" {
		t.Errorf("expected action=updated, got %q", result.Action)
	}
	if result.NewIP != "5.6.7.8" {
		t.Errorf("expected NewIP=5.6.7.8, got %q", result.NewIP)
	}
	if !fake.updateCalled {
		t.Error("UpdateIP should have been called")
	}
	if fake.updateIP != "5.6.7.8" {
		t.Errorf("UpdateIP called with %q, want 5.6.7.8", fake.updateIP)
	}
}

// TestUpdate_NoChgDNS verifies that when DNS already holds the current IP,
// we skip the upsert and the cache is refreshed.
func TestUpdate_NoChgDNS(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := baseConfig(tmpDir)
	fake := &fakeDNSClient{getIP: "5.6.7.8"}

	result, err := Update(context.Background(), cfg, Options{
		OverrideIP: "5.6.7.8",
		Client:     fake,
		Quiet:      true,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result.Action != "nochg-dns" {
		t.Errorf("expected action=nochg-dns, got %q", result.Action)
	}
	if fake.updateCalled {
		t.Error("UpdateIP should NOT be called when DNS already matches")
	}
	// Cache should have been written.
	if data, err := os.ReadFile(cfg.IPCacheFile); err != nil {
		t.Errorf("cache not written: %v", err)
	} else if !strings.Contains(string(data), "5.6.7.8") {
		t.Errorf("cache missing the new IP: %s", data)
	}
}

// TestUpdate_NoChgCache verifies that when the cache already holds the
// current IP, we short-circuit before even calling Route53.
func TestUpdate_NoChgCache(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := baseConfig(tmpDir)
	if err := writeCachedIP(cfg.IPCacheFile, "5.6.7.8"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDNSClient{getIP: "9.9.9.9"} // should never be consulted

	result, err := Update(context.Background(), cfg, Options{
		OverrideIP: "5.6.7.8",
		Client:     fake,
		Quiet:      true,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result.Action != "nochg-cache" {
		t.Errorf("expected action=nochg-cache, got %q", result.Action)
	}
	if fake.updateCalled {
		t.Error("UpdateIP should not be called on cache hit")
	}
}

// TestUpdate_DryRun verifies dry-run short-circuits without UPSERT and
// without writing the cache.
func TestUpdate_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := baseConfig(tmpDir)
	fake := &fakeDNSClient{getIP: "1.1.1.1"}

	result, err := Update(context.Background(), cfg, Options{
		OverrideIP: "5.6.7.8",
		DryRun:     true,
		Client:     fake,
		Quiet:      true,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result.Action != "dry-run" {
		t.Errorf("expected action=dry-run, got %q", result.Action)
	}
	if fake.updateCalled {
		t.Error("UpdateIP should not be called on dry-run")
	}
	if _, err := os.Stat(cfg.IPCacheFile); !os.IsNotExist(err) {
		t.Error("cache file should not be written on dry-run")
	}
}

// --- resolver.resolveIP dispatch tests ---

// newTestResolver builds a resolver whose hooks are explicit stubs.
// Passing nil for localFn / remoteFn will t.Fatal if the corresponding
// branch is taken — a guard against accidentally hitting the network.
func newTestResolver(t *testing.T, localFn func(string) (string, error), remoteFn func(context.Context) (string, error), profileName string) *resolver {
	t.Helper()
	if localFn == nil {
		localFn = func(string) (string, error) {
			t.Fatal("local resolver should not be called")
			return "", nil
		}
	}
	if remoteFn == nil {
		remoteFn = func(context.Context) (string, error) {
			t.Fatal("remote resolver should not be called")
			return "", nil
		}
	}
	return &resolver{
		localIP:  localFn,
		remoteIP: remoteFn,
		profile:  func() string { return profileName },
	}
}

func TestResolveIP_ExplicitLocal(t *testing.T) {
	var captured string
	res := newTestResolver(t,
		func(iface string) (string, error) { captured = iface; return "1.2.3.4", nil },
		nil,
		"linux")

	ip, err := res.resolveIP(context.Background(), &config.Config{
		IPSource: "local",
		Server:   &config.ServerConfig{WANInterface: "eth8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("got %s", ip)
	}
	if captured != "eth8" {
		t.Errorf("wan_interface not forwarded: got %q", captured)
	}
}

func TestResolveIP_ExplicitRemote(t *testing.T) {
	res := newTestResolver(t,
		nil,
		func(context.Context) (string, error) { return "5.6.7.8", nil },
		"udm") // even on UDM, explicit remote overrides the default

	ip, err := res.resolveIP(context.Background(), &config.Config{IPSource: "remote"})
	if err != nil {
		t.Fatal(err)
	}
	if ip != "5.6.7.8" {
		t.Errorf("got %s", ip)
	}
}

func TestResolveIP_AutoOnUDM_PicksLocal(t *testing.T) {
	res := newTestResolver(t,
		func(iface string) (string, error) {
			if iface != "" {
				t.Errorf("expected empty iface for auto-detect, got %q", iface)
			}
			return testPublicIP, nil
		},
		nil,
		"udm")

	ip, err := res.resolveIP(context.Background(), &config.Config{IPSource: ""})
	if err != nil {
		t.Fatal(err)
	}
	if ip != testPublicIP {
		t.Errorf("got %s", ip)
	}
}

func TestResolveIP_AutoOffUDM_PicksRemote(t *testing.T) {
	res := newTestResolver(t,
		nil,
		func(context.Context) (string, error) { return "5.6.7.8", nil },
		"macos")

	ip, err := res.resolveIP(context.Background(), &config.Config{IPSource: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if ip != "5.6.7.8" {
		t.Errorf("got %s", ip)
	}
}

func TestResolveIP_InvalidSource(t *testing.T) {
	res := newTestResolver(t,
		func(string) (string, error) { return "", nil },
		func(context.Context) (string, error) { return "", nil },
		"linux")

	if _, err := res.resolveIP(context.Background(), &config.Config{IPSource: "bogus"}); err == nil {
		t.Error("expected error for invalid ip_source")
	}
}

// TestUpdate_ContextTimeout verifies that a slow Route53 call is bounded
// by the caller's context deadline.
func TestUpdate_ContextTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := baseConfig(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 50_000_000) // 50ms
	defer cancel()

	_, err := Update(ctx, cfg, Options{
		OverrideIP: "5.6.7.8",
		Client:     blockingDNSClient{},
		Quiet:      true,
	})
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}
