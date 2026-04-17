package myip_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/descoped/dddns/internal/commands/myip"
)

func TestGetPublicIP(t *testing.T) {
	ip, err := myip.GetPublicIP(context.Background())
	if err != nil {
		t.Fatalf("Failed to get public IP: %v", err)
	}
	if ip == "" {
		t.Error("Expected non-empty public IP")
	}
	fmt.Printf("Public IP: %s\n", ip)
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
			err := myip.ValidatePublicIP(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}
