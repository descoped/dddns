package wanip

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// ipNet constructs a *net.IPNet from either "1.2.3.4" or "1.2.3.4/24".
func ipNet(s string) net.Addr {
	if ip, n, err := net.ParseCIDR(s); err == nil {
		return &net.IPNet{IP: ip, Mask: n.Mask}
	}
	return &net.IPNet{IP: net.ParseIP(s), Mask: net.CIDRMask(32, 32)}
}

// mockInterfaces replaces the interface lookup for the duration of the
// test. Absent interfaces produce the same "no such interface" shape as
// the real call.
func mockInterfaces(t *testing.T, byName map[string][]net.Addr) {
	t.Helper()
	orig := interfaceAddrs
	t.Cleanup(func() { interfaceAddrs = orig })
	interfaceAddrs = func(name string) ([]net.Addr, error) {
		if a, ok := byName[name]; ok {
			return a, nil
		}
		return nil, fmt.Errorf("no such interface: %s", name)
	}
}

// mockRouteFile writes content to a tempdir file and points
// defaultRoutePath at it.
func mockRouteFile(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "route")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	orig := defaultRoutePath
	t.Cleanup(func() { defaultRoutePath = orig })
	defaultRoutePath = path
}

func TestFromInterface_ReturnsPublicIPv4(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"eth8": {ipNet("81.191.174.72/24")},
	})
	ip, err := FromInterface("eth8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.String() != "81.191.174.72" {
		t.Errorf("got %s", ip)
	}
}

func TestFromInterface_SkipsPrivateFindsPublic(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"eth0": {ipNet("192.168.1.1/24"), ipNet("81.191.174.72/24")},
	})
	ip, err := FromInterface("eth0")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "81.191.174.72" {
		t.Errorf("got %s", ip)
	}
}

func TestFromInterface_SkipsCGNAT(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"eth0": {ipNet("100.64.1.1/24")}, // CGNAT-only
	})
	if _, err := FromInterface("eth0"); err == nil {
		t.Error("expected error for CGNAT-only interface")
	}
}

func TestFromInterface_LoopbackOnlyIsError(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"lo": {ipNet("127.0.0.1/8")},
	})
	if _, err := FromInterface("lo"); err == nil {
		t.Error("expected error for loopback-only interface")
	}
}

func TestFromInterface_NoAddresses(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"eth0": {},
	})
	if _, err := FromInterface("eth0"); err == nil {
		t.Error("expected error for interface with no addresses")
	}
}

func TestFromInterface_PPPoE(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"ppp0": {ipNet("81.191.174.72/32")},
	})
	ip, err := FromInterface("ppp0")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "81.191.174.72" {
		t.Errorf("got %s", ip)
	}
}

func TestFromInterface_UnknownInterface(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{"eth0": {}})
	if _, err := FromInterface("nonexistent"); err == nil {
		t.Error("expected error for unknown interface")
	}
}

func TestFromInterface_AutoDetect(t *testing.T) {
	mockInterfaces(t, map[string][]net.Addr{
		"eth8": {ipNet("81.191.174.72/24")},
	})
	mockRouteFile(t, `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
eth8	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0000A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
`)
	ip, err := FromInterface("")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "81.191.174.72" {
		t.Errorf("got %s", ip)
	}
}

func TestFromInterface_AutoDetect_NoDefaultRoute(t *testing.T) {
	mockRouteFile(t, "Iface\tDestination\n")
	if _, err := FromInterface(""); err == nil {
		t.Error("expected error when no default route present")
	}
}

func TestFromInterface_AutoDetect_MissingFile(t *testing.T) {
	orig := defaultRoutePath
	t.Cleanup(func() { defaultRoutePath = orig })
	defaultRoutePath = filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := FromInterface(""); err == nil {
		t.Error("expected error when route file is missing")
	}
}

func TestIsPublicIPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"81.191.174.72", true},
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.1.1", false},
		{"100.64.1.1", false}, // CGNAT
		{"0.0.0.0", false},
		{"224.0.0.1", false},   // multicast
		{"255.255.255.255", false}, // broadcast
		{"2001:db8::1", false}, // IPv6
	}
	for _, tt := range tests {
		got := isPublicIPv4(net.ParseIP(tt.ip))
		if got != tt.want {
			t.Errorf("isPublicIPv4(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}
