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

// GetPublicIP retrieves the public IP for current network from checkip.amazonaws.com
// and validates that it's a usable public IPv4 address. The provided ctx
// bounds the HTTP call so a SIGTERM cancels it immediately.
func GetPublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://checkip.amazonaws.com", nil)
	if err != nil {
		return "", fmt.Errorf("build public ip request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get public ip error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
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
