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

// listInterfaceNames returns the names of all up, non-loopback interfaces.
// Overridable in tests.
var listInterfaceNames = func() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		names = append(names, iface.Name)
	}
	return names, nil
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
// interface. When ifaceName is empty, auto-detect is used: first the
// default-route interface (Linux /proc/net/route), then — if that finds
// nothing (e.g. UDR7 keeps its default in a non-main routing table under
// policy-based routing) — a scan of all up interfaces for a public IPv4.
func FromInterface(ifaceName string) (net.IP, error) {
	if ifaceName == "" {
		detected, routeErr := detectDefaultRouteInterface()
		if routeErr != nil {
			ip, err := firstPublicInterfaceIP()
			if err != nil {
				return nil, fmt.Errorf("auto-detect WAN interface: %w (fallback: %v)", routeErr, err)
			}
			return ip, nil
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

// firstPublicInterfaceIP scans all up non-loopback interfaces and returns
// the first public IPv4 address it finds. Used as fallback when the route
// table has no default entry — e.g. UniFi UDR7, whose default route lives
// in a per-WAN routing table (`201.eth4`) not in `main`.
func firstPublicInterfaceIP() (net.IP, error) {
	names, err := listInterfaceNames()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		addrs, err := interfaceAddrs(name)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ip := addrToIP(a); isPublicIPv4(ip) {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface has a public IPv4 address")
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
