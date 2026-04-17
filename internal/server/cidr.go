// Package server implements the dddns HTTP listener that translates
// dyndns-style requests from UniFi's inadyn into Route53 updates.
package server

import (
	"net"
	"strings"
)

// IsAllowed reports whether remoteAddr falls within any of the supplied
// CIDR blocks. The input is typically http.Request.RemoteAddr ("host:port")
// but a bare IP is also accepted. The function fails closed:
//
//   - An unparseable host or a missing match returns false.
//   - A malformed CIDR entry is silently skipped (ServerConfig.Validate
//     rejects these upstream; skipping here is defense in depth).
//   - An empty cidrs slice returns false.
//
// Both IPv4 and IPv6 literals are supported, including the "%zone" suffix
// occasionally seen in IPv6 addresses.
func IsAllowed(remoteAddr string, cidrs []string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Callers sometimes pass just the IP. Try it directly.
		host = remoteAddr
	}
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
