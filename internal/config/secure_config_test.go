package config_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
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
			Bind:         "127.0.0.1:53353",
			SharedSecret: "super-secret-value",
			AllowedCIDRs: []string{"127.0.0.0/8", "192.168.1.0/24"},
			AuditLog:     "/var/log/dddns-audit.log",
			WANInterface: "eth8",
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

// TestSaveSecure_EnforcesSecurePermsOnWrite guards the security boundary:
// SaveSecure must leave the on-disk file at 0400 so no other local user
// can read the encrypted vault for offline attack.
func TestSaveSecure_EnforcesSecurePermsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.secure")

	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
	if err := config.SaveSecure(cfg, path); err != nil {
		t.Fatalf("SaveSecure: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != constants.SecureConfigPerm {
		t.Errorf("secure config perms = %04o, want %04o", mode, constants.SecureConfigPerm)
	}
}

// TestLoadSecure_RejectsWorldReadablePerms mirrors the plaintext guard
// — even an encrypted file at 0644 is not loaded, because allowing it
// would signal that world-readable vaults are acceptable practice.
func TestLoadSecure_RejectsWorldReadablePerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.secure")

	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
	if err := config.SaveSecure(cfg, path); err != nil {
		t.Fatalf("SaveSecure: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, err := config.LoadSecure(path); err == nil {
		t.Fatal("LoadSecure accepted 0644 secure config; expected rejection")
	}
}

// TestSaveSecure_OverwriteChmodsBackTo0400 covers the "rotate-secret
// over existing 0400 file" flow. SaveSecure temporarily chmods to 0600
// so the write can truncate; a regression leaving the file at 0600
// would silently weaken the security boundary after a rotation.
func TestSaveSecure_OverwriteChmodsBackTo0400(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.secure")

	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
	if err := config.SaveSecure(cfg, path); err != nil {
		t.Fatalf("first SaveSecure: %v", err)
	}

	cfg.TTL = 600
	if err := config.SaveSecure(cfg, path); err != nil {
		t.Fatalf("second SaveSecure over existing 0400 file: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != constants.SecureConfigPerm {
		t.Errorf("perms after overwrite = %04o, want %04o", mode, constants.SecureConfigPerm)
	}

	loaded, err := config.LoadSecure(path)
	if err != nil {
		t.Fatalf("LoadSecure after overwrite: %v", err)
	}
	if loaded.TTL != 600 {
		t.Errorf("re-saved TTL not reflected: got %d, want 600", loaded.TTL)
	}
}

// TestLoadSecure_RejectsTamperedCredentialsVault is the AES-GCM auth-tag
// guard for aws_credentials_vault (complement to the existing
// secret_vault tamper test). Flipping the last byte of the decoded
// ciphertext must surface as a decrypt error, not silent garbage.
func TestLoadSecure_RejectsTamperedCredentialsVault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.secure")

	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
	if err := config.SaveSecure(cfg, path); err != nil {
		t.Fatalf("SaveSecure: %v", err)
	}

	if err := os.Chmod(path, constants.ConfigFilePerm); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tampered := flipLastByteOfVault(t, string(raw), "aws_credentials_vault:")
	if err := os.WriteFile(path, []byte(tampered), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write tampered: %v", err)
	}
	if err := os.Chmod(path, constants.SecureConfigPerm); err != nil {
		t.Fatalf("chmod back: %v", err)
	}

	_, err = config.LoadSecure(path)
	if err == nil {
		t.Fatal("LoadSecure accepted tampered aws_credentials_vault; GCM auth tag should reject it")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "decrypt") {
		t.Errorf("error should cite decrypt failure, got: %v", err)
	}
}

// flipLastByteOfVault decodes the base64 value of the YAML line starting
// with prefix, flips its last byte, and returns the re-encoded YAML.
func flipLastByteOfVault(t *testing.T, yaml, prefix string) string {
	t.Helper()
	lines := strings.Split(yaml, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		idx := strings.Index(line, ":")
		value := strings.TrimSpace(line[idx+1:])
		value = strings.Trim(value, "\"")

		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatalf("decode vault: %v", err)
		}
		if len(decoded) == 0 {
			t.Fatalf("%s is empty", prefix)
		}
		decoded[len(decoded)-1] ^= 0xFF
		lines[i] = line[:idx+1] + " " + base64.StdEncoding.EncodeToString(decoded)
		return strings.Join(lines, "\n")
	}
	t.Fatalf("line starting with %q not found in YAML:\n%s", prefix, yaml)
	return yaml
}

// TestMigrateToSecure_WipesPlaintext guards the "migration leaves no
// plaintext on disk" contract. A silently-failed wipe would leave the
// operator with plaintext AWS credentials on a filesystem they now
// believe is encrypted.
func TestMigrateToSecure_WipesPlaintext(t *testing.T) {
	dir := t.TempDir()
	plaintextPath := filepath.Join(dir, "config.yaml")
	securePath := filepath.Join(dir, "config.secure")

	cfg := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
		IPCacheFile:  filepath.Join(dir, "last-ip.txt"),
	}
	content := config.FormatConfigYAML(cfg)
	if err := os.WriteFile(plaintextPath, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	config.SetActivePath(plaintextPath)
	t.Cleanup(func() { config.SetActivePath("") })

	if err := config.MigrateToSecure(plaintextPath, securePath); err != nil {
		t.Fatalf("MigrateToSecure: %v", err)
	}

	if _, err := os.Stat(plaintextPath); !os.IsNotExist(err) {
		t.Errorf("plaintext config still exists after migration: err=%v", err)
	}
	if _, err := os.Stat(securePath); err != nil {
		t.Errorf("secure config missing after migration: %v", err)
	}
}
