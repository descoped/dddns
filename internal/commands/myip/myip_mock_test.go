package myip

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockAPI spins up an httptest server that serves a single fixed body
// and temporarily replaces ipAPIBaseURL with its URL. The returned cleanup
// function restores the original URL and shuts the server down.
func newMockAPI(t *testing.T, body string) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	origURL := ipAPIBaseURL
	ipAPIBaseURL = srv.URL
	return func() {
		ipAPIBaseURL = origURL
		srv.Close()
	}
}

func TestIsProxyIP_Success_NotProxy(t *testing.T) {
	defer newMockAPI(t, `{"status":"success","proxy":false}`)()

	ip := "1.2.3.4"
	isProxy, err := IsProxyIP(&ip)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isProxy {
		t.Error("expected proxy=false, got true")
	}
}

func TestIsProxyIP_Success_IsProxy(t *testing.T) {
	defer newMockAPI(t, `{"status":"success","proxy":true}`)()

	ip := "1.2.3.4"
	isProxy, err := IsProxyIP(&ip)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isProxy {
		t.Error("expected proxy=true, got false")
	}
}

func TestIsProxyIP_StatusFail(t *testing.T) {
	defer newMockAPI(t, `{"status":"fail","message":"invalid query"}`)()

	ip := "1.2.3.4"
	_, err := IsProxyIP(&ip)
	if err == nil {
		t.Fatal("expected error when status is fail, got nil")
	}
	if !strings.Contains(err.Error(), "invalid query") {
		t.Errorf("expected error to mention the API message, got: %v", err)
	}
}

func TestIsProxyIP_MalformedJSON(t *testing.T) {
	defer newMockAPI(t, `{not valid json`)()

	ip := "1.2.3.4"
	_, err := IsProxyIP(&ip)
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestIsProxyIP_EmptyStatus(t *testing.T) {
	// Body omits the status field entirely (zero value after unmarshal).
	// Must be treated as a failure, not silently read proxy=false.
	defer newMockAPI(t, `{"proxy":false}`)()

	ip := "1.2.3.4"
	_, err := IsProxyIP(&ip)
	if err == nil {
		t.Fatal("expected error on missing status, got nil")
	}
}
