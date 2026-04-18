package myip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// httpClient with timeout for reliability in cron jobs
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// checkipURL is the endpoint consulted by GetPublicIP. Overridable in
// tests via httptest.NewServer; production callers must not change it.
var checkipURL = "https://checkip.amazonaws.com"

// GetPublicIP retrieves the public IP for current network from checkip.amazonaws.com
// and validates that it's a usable public IPv4 address. The provided ctx
// bounds the HTTP call so a SIGTERM cancels it immediately.
func GetPublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkipURL, nil)
	if err != nil {
		return "", fmt.Errorf("build public ip request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get public ip error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checkip returned HTTP %d", resp.StatusCode)
	}

	// checkip.amazonaws.com returns a bare IPv4 address plus a trailing
	// newline (~16 bytes). Cap the read at 64 bytes so a hostile endpoint
	// can't exhaust memory by streaming megabytes into a string allocation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", fmt.Errorf("failed to read public ip response: %w", err)
	}

	ip := strings.TrimSpace(string(body))
	if err := ValidatePublicIP(ip); err != nil {
		return "", fmt.Errorf("checkip returned unusable IP: %w", err)
	}
	return ip, nil
}

// ValidatePublicIP rejects addresses that are not suitable for a public DNS
// A record: malformed, IPv6, loopback, link-local, unspecified, multicast, or
// private (RFC1918). Returns nil when the string parses to a publicly-
// routable unicast IPv4 address.
func ValidatePublicIP(ip string) error {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP address: %q", ip)
	}
	if parsed.To4() == nil {
		return fmt.Errorf("IPv6 not supported (got %q); dddns is IPv4-only", ip)
	}
	if !parsed.IsGlobalUnicast() {
		return fmt.Errorf("IP is not globally unicast: %q", ip)
	}
	if parsed.IsPrivate() {
		return fmt.Errorf("IP is in a private (RFC1918) range: %q", ip)
	}
	return nil
}
