// Package wanip resolves the router's current public IPv4 address by
// reading the WAN interface directly from the OS, avoiding the round
// trip to checkip.amazonaws.com.
package wanip

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// defaultRoutePath is the Linux proc file parsed for the default-route
// interface name. Overridable in tests.
var defaultRoutePath = "/proc/net/route"

// interfaceAddrs returns the addresses bound to an interface. Overridable
// in tests.
var interfaceAddrs = func(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}

// cgnat is RFC6598 shared address space used for carrier-grade NAT.
// Syntactically global unicast but not publicly routable.
var cgnat = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// FromInterface returns the first usable public IPv4 address on the given
// interface. When ifaceName is empty, the interface on the default route
// is used (Linux only — reads /proc/net/route).
func FromInterface(ifaceName string) (net.IP, error) {
	if ifaceName == "" {
		detected, err := detectDefaultRouteInterface()
		if err != nil {
			return nil, fmt.Errorf("auto-detect WAN interface: %w", err)
		}
		ifaceName = detected
	}

	addrs, err := interfaceAddrs(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface %q: %w", ifaceName, err)
	}

	for _, a := range addrs {
		if ip := addrToIP(a); isPublicIPv4(ip) {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("interface %q has no public IPv4 address", ifaceName)
}

// detectDefaultRouteInterface reads /proc/net/route and returns the
// Iface column for the 0.0.0.0 destination row.
func detectDefaultRouteInterface() (string, error) {
	f, err := os.Open(defaultRoutePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return "", fmt.Errorf("empty route table at %s", defaultRoutePath)
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if fields[1] == "00000000" {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no default route in %s", defaultRoutePath)
}

// addrToIP extracts a net.IP from a net.Addr (typically *net.IPNet).
// Returns nil for unrecognised concrete types.
func addrToIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

// isPublicIPv4 reports whether ip is a globally-routable public IPv4
// address: not loopback, not link-local, not multicast, not unspecified,
// not RFC1918 private, not CGNAT, not IPv6.
func isPublicIPv4(ip net.IP) bool {
	if ip == nil || ip.To4() == nil {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip.IsPrivate() {
		return false
	}
	if cgnat.Contains(ip) {
		return false
	}
	return true
}
