package server

import "testing"

func TestIsAllowed(t *testing.T) {
	defaultCIDRs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	tests := []struct {
		name       string
		remoteAddr string
		cidrs      []string
		want       bool
	}{
		// loopback / RFC1918 — expected allow
		{"loopback v4 with port", "127.0.0.1:54321", defaultCIDRs, true},
		{"loopback v4 without port", "127.0.0.1", defaultCIDRs, true},
		{"rfc1918 10/8", "10.1.2.3:54321", defaultCIDRs, true},
		{"rfc1918 172.16/12", "172.20.1.1:54321", defaultCIDRs, true},
		{"rfc1918 192.168/16", "192.168.1.5:54321", defaultCIDRs, true},

		// public — expected deny
		{"public 8.8.8.8", "8.8.8.8:54321", defaultCIDRs, false},
		{"public 1.1.1.1", "1.1.1.1:54321", defaultCIDRs, false},

		// edge cases — expected deny
		{"empty remoteAddr", "", defaultCIDRs, false},
		{"empty cidrs", "127.0.0.1:1234", nil, false},
		{"malformed host", "not-an-ip:1234", defaultCIDRs, false},

		// narrower subnet
		{"narrow subnet match", "192.168.1.7:1234", []string{"192.168.1.0/24"}, true},
		{"narrow subnet miss", "192.168.2.7:1234", []string{"192.168.1.0/24"}, false},

		// ipv6
		{"ipv6 loopback in ::1/128", "[::1]:1234", []string{"::1/128"}, true},
		{"ipv6 public not in ::1/128", "[2606:4700:4700::1111]:1234", []string{"::1/128"}, false},

		// ipv6 with zone
		{"ipv6 link-local with zone", "[fe80::1%eth0]:1234", []string{"fe80::/10"}, true},

		// malformed CIDR in list is ignored; valid CIDR still matches
		{"malformed cidr is skipped", "127.0.0.1:1234", []string{"not-a-cidr", "127.0.0.0/8"}, true},
		{"only malformed cidrs", "127.0.0.1:1234", []string{"foo", "bar"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAllowed(tt.remoteAddr, tt.cidrs)
			if got != tt.want {
				t.Errorf("IsAllowed(%q, %v) = %v, want %v", tt.remoteAddr, tt.cidrs, got, tt.want)
			}
		})
	}
}
