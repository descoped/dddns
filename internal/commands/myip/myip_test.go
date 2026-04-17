package myip_test

import (
	"fmt"
	"testing"

	"github.com/descoped/dddns/internal/commands/myip"
)

func TestGetPublicIP(t *testing.T) {
	ip, err := myip.GetPublicIP()
	if err != nil {
		t.Fatalf("Failed to get public ip: %s", err)
	}
	if ip == "" {
		t.Error("Expected non-empty public IP")
	}
	fmt.Printf("Public IP: %s\n", ip)
}

func TestIsProxyIP_InvalidIP(t *testing.T) {
	invalidIP := "not-an-ip"
	_, err := myip.IsProxyIP(&invalidIP)

	// ip-api.com returns {"status":"fail","message":"invalid query"} for
	// malformed inputs. That is now surfaced as an error instead of being
	// silently collapsed to "not a proxy".
	if err == nil {
		t.Error("Expected error for invalid IP, got nil")
	}
}

func TestIsProxyIP_NilIP(t *testing.T) {
	_, err := myip.IsProxyIP(nil)

	// Should return an error for nil IP
	if err == nil {
		t.Error("Expected error for nil IP, got nil")
	}
}
