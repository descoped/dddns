package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
)

// TestSaveLoadSecure_WithServerBlock exercises a full round-trip of a
// Config containing a Server block: Save encrypts the shared secret into
// secret_vault, Load decrypts it back into SharedSecret.
func TestSaveLoadSecure_WithServerBlock(t *testing.T) {
	tmpDir := t.TempDir()
	securePath := filepath.Join(tmpDir, "config.secure")

	in := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		IPCacheFile:  filepath.Join(tmpDir, "cache.txt"),
		IPSource:     "local",
		Server: &config.ServerConfig{
			Bind:          "127.0.0.1:53353",
			SharedSecret:  "super-secret-value",
			AllowedCIDRs:  []string{"127.0.0.0/8", "192.168.1.0/24"},
			AuditLog:      "/var/log/dddns-audit.log",
			OnAuthFailure: "logger",
			WANInterface:  "eth8",
		},
	}

	if err := config.SaveSecure(in, securePath); err != nil {
		t.Fatalf("SaveSecure failed: %v", err)
	}

	out, err := config.LoadSecure(securePath)
	if err != nil {
		t.Fatalf("LoadSecure failed: %v", err)
	}

	// Top-level fields.
	if out.AWSAccessKey != in.AWSAccessKey || out.AWSSecretKey != in.AWSSecretKey {
		t.Errorf("AWS creds did not round-trip")
	}
	if out.IPSource != "local" {
		t.Errorf("IPSource = %q, want local", out.IPSource)
	}

	// Server block.
	if out.Server == nil {
		t.Fatal("Server was not restored")
	}
	if out.Server.SharedSecret != "super-secret-value" {
		t.Errorf("SharedSecret mismatch: got %q", out.Server.SharedSecret)
	}
	if out.Server.Bind != in.Server.Bind {
		t.Errorf("Bind mismatch")
	}
	if len(out.Server.AllowedCIDRs) != 2 {
		t.Errorf("AllowedCIDRs length mismatch: got %d", len(out.Server.AllowedCIDRs))
	}
	if out.Server.WANInterface != "eth8" {
		t.Errorf("WANInterface mismatch")
	}
}

// TestSaveSecure_SecretIsEncryptedAtRest verifies that reading the on-disk
// .secure file as plain text does not reveal the shared secret. The vault
// should contain only the base64 ciphertext.
func TestSaveSecure_SecretIsEncryptedAtRest(t *testing.T) {
	tmpDir := t.TempDir()
	securePath := filepath.Join(tmpDir, "config.secure")

	in := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:53353",
			SharedSecret: "distinctive-plaintext-marker-xyzzy",
			AllowedCIDRs: []string{"127.0.0.0/8"},
		},
	}
	if err := config.SaveSecure(in, securePath); err != nil {
		t.Fatalf("SaveSecure failed: %v", err)
	}

	raw, err := os.ReadFile(securePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "distinctive-plaintext-marker-xyzzy") {
		t.Error("plaintext shared secret appears in the .secure file — encryption bypassed")
	}
	// Sanity: AWS creds must also be absent in plaintext.
	if strings.Contains(string(raw), "AKIATEST") || strings.Contains(string(raw), "SECRETTEST") {
		t.Error("plaintext AWS credentials appear in the .secure file")
	}
}

// TestLoadSecure_NoServerBlock verifies that an existing .secure file
// predating this change (no server block) still loads cleanly and leaves
// Server as nil.
func TestLoadSecure_NoServerBlock(t *testing.T) {
	tmpDir := t.TempDir()
	securePath := filepath.Join(tmpDir, "config.secure")

	in := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		// No Server, no IPSource.
	}
	if err := config.SaveSecure(in, securePath); err != nil {
		t.Fatalf("SaveSecure failed: %v", err)
	}

	out, err := config.LoadSecure(securePath)
	if err != nil {
		t.Fatalf("LoadSecure failed: %v", err)
	}
	if out.Server != nil {
		t.Errorf("Server should be nil, got %+v", out.Server)
	}
	if out.IPSource != "" {
		t.Errorf("IPSource should be empty, got %q", out.IPSource)
	}
}

// TestLoadSecure_TamperedVault verifies that flipping a byte of the
// secret_vault field causes LoadSecure to fail (GCM authentication).
func TestLoadSecure_TamperedVault(t *testing.T) {
	tmpDir := t.TempDir()
	securePath := filepath.Join(tmpDir, "config.secure")

	in := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:53353",
			SharedSecret: "will-be-corrupted",
			AllowedCIDRs: []string{"127.0.0.0/8"},
		},
	}
	if err := config.SaveSecure(in, securePath); err != nil {
		t.Fatal(err)
	}

	// Corrupt: prepend "AAAA" to the secret_vault value so the decoded
	// bytes shift and GCM authentication fails.
	raw, err := os.ReadFile(securePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), "secret_vault: ", "secret_vault: AAAA", 1)
	if tampered == string(raw) {
		t.Fatal("could not find secret_vault line to corrupt")
	}
	// SaveSecure writes the file read-only (0400); chmod it before
	// overwriting.
	if err := os.Chmod(securePath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(securePath, []byte(tampered), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := config.LoadSecure(securePath); err == nil {
		t.Error("expected LoadSecure to fail on tampered secret_vault, got nil")
	}
}
