package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
