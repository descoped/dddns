package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/spf13/cobra"
)

// newStubServer spins up an httptest server that returns the supplied
// status + body on every request.
func newStubServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestServeTest_Good(t *testing.T) {
	ts := newStubServer(t, http.StatusOK, "good 1.2.3.4\n")
	defer ts.Close()

	var buf bytes.Buffer
	if err := performServeTest(ts.URL, "test.example.com", "secret", "1.2.3.4", &buf); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if !strings.Contains(buf.String(), "HTTP 200") || !strings.Contains(buf.String(), "good 1.2.3.4") {
		t.Errorf("output missing expected fields:\n%s", buf.String())
	}
}

func TestServeTest_NoChange(t *testing.T) {
	ts := newStubServer(t, http.StatusOK, "nochg 1.2.3.4\n")
	defer ts.Close()

	var buf bytes.Buffer
	if err := performServeTest(ts.URL, "test.example.com", "secret", "1.2.3.4", &buf); err != nil {
		t.Errorf("expected nil for nochg, got %v", err)
	}
}

func TestServeTest_BadAuth(t *testing.T) {
	ts := newStubServer(t, http.StatusOK, "badauth\n")
	defer ts.Close()

	var buf bytes.Buffer
	err := performServeTest(ts.URL, "test.example.com", "wrong", "1.2.3.4", &buf)
	if err == nil {
		t.Error("expected error for badauth body")
	}
}

func TestServeTest_CIDRDeny403(t *testing.T) {
	ts := newStubServer(t, http.StatusForbidden, "")
	defer ts.Close()

	var buf bytes.Buffer
	err := performServeTest(ts.URL, "test.example.com", "secret", "1.2.3.4", &buf)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestServeTest_Dnserr(t *testing.T) {
	ts := newStubServer(t, http.StatusOK, "dnserr\n")
	defer ts.Close()

	var buf bytes.Buffer
	err := performServeTest(ts.URL, "test.example.com", "secret", "1.2.3.4", &buf)
	if err == nil {
		t.Error("expected error for dnserr body")
	}
}

func TestServeTest_NetworkError(t *testing.T) {
	// Point at an address nothing is listening on (port 1 is privileged
	// and unlikely to be bound by userland).
	var buf bytes.Buffer
	err := performServeTest("http://127.0.0.1:1", "test.example.com", "secret", "1.2.3.4", &buf)
	if err == nil {
		t.Error("expected error for unreachable target")
	}
}

// TestRunServeTest_RejectsMissingServerBlock covers the fail-closed
// contract: running `dddns serve test` against a config without a
// server block must return a clear diagnostic, not panic on a nil
// cfg.Server.
func TestRunServeTest_RejectsMissingServerBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"hostname: \"test.example.com\"\n" +
		"ttl: 300\n" +
		"ip_cache_file: \"" + filepath.Join(dir, "last-ip.txt") + "\"\n"
	if err := os.WriteFile(path, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	config.SetActivePath(path)
	t.Cleanup(func() { config.SetActivePath("") })

	origHost, origIP := serveTestHostname, serveTestIP
	t.Cleanup(func() {
		serveTestHostname = origHost
		serveTestIP = origIP
	})
	serveTestHostname = ""
	serveTestIP = "203.0.113.10"

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	err := runServeTest(cmd, nil)
	if err == nil {
		t.Fatal("runServeTest accepted config with no server block")
	}
	if !strings.Contains(err.Error(), "serve mode not configured") {
		t.Errorf("error should cite missing server block, got: %v", err)
	}
}

func TestLoopbackURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"127.0.0.1:53353", "http://127.0.0.1:53353"},
		{"0.0.0.0:53353", "http://127.0.0.1:53353"},
		{":53353", "http://127.0.0.1:53353"},
		{"192.168.1.5:53353", "http://192.168.1.5:53353"},
		{"[::]:53353", "http://127.0.0.1:53353"},
	}
	for _, tt := range tests {
		if got := loopbackURL(tt.in); got != tt.want {
			t.Errorf("loopbackURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
