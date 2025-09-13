package myip_test

import (
	"fmt"
	"testing"

	"github.com/descoped/dddns/internal/commands/myip"
)

func TestGetPublicIP(t *testing.T) {
	// get ip
	ip, err := myip.GetPublicIP()
	if err != nil {
		t.Errorf("Failed to get public ip: %s", err)
	}

	// check if ip is public
	isProxyIP, err := myip.IsProxyIP(&ip)
	if err != nil {
		t.Errorf("Failed to check if public ip is a proxy ip: %s", ip)
	}

	fmt.Printf("Is proxy: %s => %t\n", ip, isProxyIP)
}

func TestIsProxyIP_InvalidIP(t *testing.T) {
	invalidIP := "not-an-ip"
	isProxy, err := myip.IsProxyIP(&invalidIP)

	// ip-api.com accepts any string, so we just check it returns false
	if err != nil {
		t.Errorf("Unexpected error for invalid IP: %v", err)
	}
	if isProxy {
		t.Error("Invalid IP should not be detected as proxy")
	}
}

func TestIsProxyIP_NilIP(t *testing.T) {
	_, err := myip.IsProxyIP(nil)

	// Should return an error for nil IP
	if err == nil {
		t.Error("Expected error for nil IP, got nil")
	}
}
