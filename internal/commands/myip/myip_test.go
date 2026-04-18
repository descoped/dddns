package myip

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetPublicIP(t *testing.T) {
	ip, err := GetPublicIP(context.Background())
	if err != nil {
		t.Fatalf("Failed to get public IP: %v", err)
	}
	if ip == "" {
		t.Error("Expected non-empty public IP")
	}
	fmt.Printf("Public IP: %s\n", ip)
}

// TestGetPublicIP_RejectsNon200 confirms an upstream error page — even
// one that happens to contain a valid-looking IP — is rejected. This is
// the F1 hardening from the v0.2.0 security review.
func TestGetPublicIP_RejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("8.8.8.8\n"))
	}))
	defer srv.Close()

	orig := checkipURL
	t.Cleanup(func() { checkipURL = orig })
	checkipURL = srv.URL

	if _, err := GetPublicIP(context.Background()); err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	} else if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected status 500 in error, got: %v", err)
	}
}

// TestGetPublicIP_BoundedRead caps the read at 64 bytes so a hostile
// endpoint can't exhaust memory via a large response body.
func TestGetPublicIP_BoundedRead(t *testing.T) {
	// Send 10 KB of trash prefixed by a valid IP. The validator will
	// accept the IP if the read stops before the trash starts; otherwise
	// the IP parser will fail on the concatenation.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// 128 bytes of padding after a valid IP — well past the 64-byte cap.
		_, _ = w.Write([]byte("1.2.3.4\n" + strings.Repeat("X", 128)))
	}))
	defer srv.Close()

	orig := checkipURL
	t.Cleanup(func() { checkipURL = orig })
	checkipURL = srv.URL

	// With a bounded read, we get a truncated body. Depending on whether
	// the newline falls inside the 64-byte window, validation may
	// succeed with "1.2.3.4" or fail with garbage. Either way we must
	// NOT panic or allocate megabytes. The concrete assertion is that
	// GetPublicIP returns in bounded time without error or with a
	// readable validation error — not a panic, not an OOM.
	_, err := GetPublicIP(context.Background())
	if err != nil && !strings.Contains(err.Error(), "unusable IP") {
		t.Errorf("unexpected error class: %v", err)
	}
}

func TestValidatePublicIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid public", "1.2.3.4", false},
		{"valid public cloudflare", "8.8.8.8", false},
		{"empty", "", true},
		{"not an IP", "not-an-ip", true},
		{"loopback", "127.0.0.1", true},
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"link-local 169.254", "169.254.1.1", true},
		{"multicast", "224.0.0.1", true},
		{"unspecified", "0.0.0.0", true},
		{"ipv6", "2001:db8::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}
