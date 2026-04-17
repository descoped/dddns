package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/updater"
)

// Test fixtures. Single source of truth for placeholders used across every
// server-package test. testPublicIP is RFC 5737 TEST-NET-3; testHostname is
// RFC 2606 reserved. Change here to change everywhere.
const (
	testPublicIP = "203.0.113.42"
	testHostname = "home.example.com"
	testSecretV  = "correct-horse-battery-staple"
)

// fixture wires a Handler with in-memory dependencies and exposes
// overrideable stubs for the upstream calls. Each test gets its own.
type fixture struct {
	handler   *Handler
	auth      *Authenticator
	audit     *AuditLog
	status    *StatusWriter
	auditPath string
	statusPath string

	wanIPReturn    net.IP
	wanIPErr       error
	updaterResult  *updater.Result
	updaterErr     error
	updaterCalled  bool
	updaterOpts    updater.Options
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	tmp := t.TempDir()
	f := &fixture{
		auditPath:  filepath.Join(tmp, "audit.log"),
		statusPath: filepath.Join(tmp, "status.json"),
		wanIPReturn: net.ParseIP(testPublicIP),
	}
	cfg := &config.Config{
		Hostname: testHostname,
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:53353",
			SharedSecret: testSecretV,
			AllowedCIDRs: []string{"127.0.0.0/8", "192.168.0.0/16"},
		},
	}
	f.auth = NewAuthenticator(testSecretV)
	f.audit = NewAuditLog(f.auditPath)
	f.status = NewStatusWriter(f.statusPath)
	h := NewHandler(cfg, f.auth, f.audit, f.status)
	h.wanIP = func(string) (net.IP, error) { return f.wanIPReturn, f.wanIPErr }
	h.updateIP = func(_ context.Context, _ *config.Config, opts updater.Options) (*updater.Result, error) {
		f.updaterCalled = true
		f.updaterOpts = opts
		return f.updaterResult, f.updaterErr
	}
	f.handler = h
	return f
}

// do executes a request against the handler. remote overrides
// req.RemoteAddr (httptest doesn't set a realistic one by default).
func (f *fixture) do(req *http.Request, remote string) *httptest.ResponseRecorder {
	if remote != "" {
		req.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

// newReq builds a GET with Basic Auth and the given query params.
func newReq(t *testing.T, params map[string]string, password string) *http.Request {
	t.Helper()
	u := "/nic/update"
	sep := "?"
	for k, v := range params {
		u += sep + k + "=" + v
		sep = "&"
	}
	req := httptest.NewRequest(http.MethodGet, u, nil)
	if password != "" {
		req.SetBasicAuth("dddns", password)
	}
	return req
}

// --- §10 response mapping ---

func TestHandler_CIDRDeny(t *testing.T) {
	f := newFixture(t)
	req := newReq(t, map[string]string{"hostname": testHostname, "myip": "1.2.3.4"}, testSecretV)
	w := f.do(req, "8.8.8.8:54321") // public IP, not in allowlist
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body should be empty on CIDR deny, got %q", w.Body.String())
	}
}

func TestHandler_MethodNotGet(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/nic/update", nil)
	req.SetBasicAuth("dddns", testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandler_MissingAuth(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/nic/update?hostname="+testHostname, nil)
	w := f.do(req, "127.0.0.1:54321")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "badauth" {
		t.Errorf("body = %q, want badauth", got)
	}
}

func TestHandler_BadPassword(t *testing.T) {
	f := newFixture(t)
	req := newReq(t, map[string]string{"hostname": testHostname}, "wrong")
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "badauth" {
		t.Errorf("body = %q, want badauth", got)
	}
}

func TestHandler_MissingHostname(t *testing.T) {
	f := newFixture(t)
	req := newReq(t, nil, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "notfqdn" {
		t.Errorf("body = %q, want notfqdn", got)
	}
}

func TestHandler_WrongHostname(t *testing.T) {
	f := newFixture(t)
	req := newReq(t, map[string]string{"hostname": "other.example.com"}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "nohost" {
		t.Errorf("body = %q, want nohost", got)
	}
}

func TestHandler_UpdatedGood(t *testing.T) {
	f := newFixture(t)
	f.updaterResult = &updater.Result{Action: "updated", NewIP: testPublicIP, Hostname: testHostname}
	req := newReq(t, map[string]string{"hostname": testHostname, "myip": testPublicIP}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "good " + testPublicIP {
		t.Errorf("body = %q", got)
	}
	if !f.updaterCalled {
		t.Error("updater should have been called")
	}
	if f.updaterOpts.OverrideIP != testPublicIP {
		t.Errorf("OverrideIP = %q, want local WAN IP", f.updaterOpts.OverrideIP)
	}
}

func TestHandler_NoChange(t *testing.T) {
	f := newFixture(t)
	f.updaterResult = &updater.Result{Action: "nochg-cache", NewIP: testPublicIP}
	req := newReq(t, map[string]string{"hostname": testHostname}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "nochg " + testPublicIP {
		t.Errorf("body = %q", got)
	}
}

func TestHandler_Route53Error(t *testing.T) {
	f := newFixture(t)
	f.updaterErr = fmt.Errorf("route53: throttled")
	req := newReq(t, map[string]string{"hostname": testHostname}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "dnserr" {
		t.Errorf("body = %q, want dnserr", got)
	}
}

func TestHandler_WANIPError(t *testing.T) {
	f := newFixture(t)
	f.wanIPErr = fmt.Errorf("interface not found")
	f.wanIPReturn = nil
	req := newReq(t, map[string]string{"hostname": testHostname}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "dnserr" {
		t.Errorf("body = %q, want dnserr", got)
	}
}

func TestHandler_LockoutResponds_Badauth(t *testing.T) {
	f := newFixture(t)
	// Trip the lockout with enough failures.
	for i := 0; i < MaxFailuresPerWindow; i++ {
		f.auth.Check("wrong")
	}
	// Correct password now.
	req := newReq(t, map[string]string{"hostname": testHostname}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")
	if got := strings.TrimSpace(w.Body.String()); got != "badauth" {
		t.Errorf("body = %q, want badauth (locked)", got)
	}
}

// --- anomaly and side-effect checks ---

// TestHandler_MyipAnomalyLoggedButLocalIPUsed verifies that when the
// myip query param disagrees with the local WAN IP, the handler still
// pushes the local IP and records the claim for the audit trail.
func TestHandler_MyipAnomalyLoggedButLocalIPUsed(t *testing.T) {
	f := newFixture(t)
	f.updaterResult = &updater.Result{Action: "updated", NewIP: testPublicIP}
	req := newReq(t, map[string]string{"hostname": testHostname, "myip": "9.9.9.9"}, testSecretV)
	w := f.do(req, "127.0.0.1:54321")

	// Response advertises the real IP, not the claim.
	if !strings.Contains(w.Body.String(), testPublicIP) {
		t.Errorf("body should contain local IP, got %q", w.Body.String())
	}
	// Updater called with local IP, ignoring the claim.
	if f.updaterOpts.OverrideIP != testPublicIP {
		t.Errorf("OverrideIP = %q, want local IP", f.updaterOpts.OverrideIP)
	}
	// Audit entry records both claimed and verified.
	raw, err := os.ReadFile(f.auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(raw[:len(raw)-1], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.MyIPClaimed != "9.9.9.9" || entry.MyIPVerified != testPublicIP {
		t.Errorf("audit mismatch: claimed=%q verified=%q", entry.MyIPClaimed, entry.MyIPVerified)
	}
}

// TestHandler_WritesStatus verifies the status.json file is refreshed
// on every request with the expected fields.
func TestHandler_WritesStatus(t *testing.T) {
	f := newFixture(t)
	fixed := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.handler.now = func() time.Time { return fixed }
	f.updaterResult = &updater.Result{Action: "updated", NewIP: testPublicIP}

	req := newReq(t, map[string]string{"hostname": testHostname}, testSecretV)
	f.do(req, "127.0.0.1:54321")

	raw, err := os.ReadFile(f.statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var snap StatusSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if !snap.LastRequestAt.Equal(fixed) {
		t.Errorf("LastRequestAt = %v, want %v", snap.LastRequestAt, fixed)
	}
	if snap.LastAuthOutcome != "ok" {
		t.Errorf("LastAuthOutcome = %q", snap.LastAuthOutcome)
	}
	if snap.LastAction != "updated" {
		t.Errorf("LastAction = %q", snap.LastAction)
	}
}

// TestStatusWriter_ReadRoundTrip verifies that Write followed by
// ReadStatus returns the same snapshot.
func TestStatusWriter_ReadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "status.json")
	w := NewStatusWriter(path)

	in := StatusSnapshot{
		LastRequestAt:   time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
		LastRemoteAddr:  "127.0.0.1:54321",
		LastAuthOutcome: "ok",
		LastAction:      "updated",
	}
	if err := w.Write(in); err != nil {
		t.Fatal(err)
	}

	got, err := ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}
	if !got.LastRequestAt.Equal(in.LastRequestAt) {
		t.Errorf("LastRequestAt mismatch: got %v want %v", got.LastRequestAt, in.LastRequestAt)
	}
	if got.LastRemoteAddr != in.LastRemoteAddr {
		t.Errorf("LastRemoteAddr mismatch")
	}
	if got.LastAction != in.LastAction {
		t.Errorf("LastAction mismatch")
	}
}

func TestReadStatus_MissingFile(t *testing.T) {
	if _, err := ReadStatus(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadStatus_Malformed(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "status.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStatus(path); err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// TestStatusWriter_Atomic verifies a concurrent read never sees a
// partially-written file. The writer goroutine is joined before the
// test returns so t.TempDir's cleanup doesn't race an in-flight
// os.CreateTemp.
func TestStatusWriter_Atomic(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "status.json")
	w := NewStatusWriter(path)

	const n = 200
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			_ = w.Write(StatusSnapshot{
				LastRequestAt: time.Now(),
				LastAction:    fmt.Sprintf("iter-%d", i),
			})
		}
	}()

	// Concurrent readers — every parse must succeed.
	for i := 0; i < n/2; i++ {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) == 0 {
			continue
		}
		var snap StatusSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			t.Errorf("reader parsed invalid JSON at iter %d: %v\n%s", i, err, raw)
		}
	}
	<-done // block until the writer finishes so TempDir cleanup is safe
}
