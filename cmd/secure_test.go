package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
)

// TestEnableSecure_RoundTripPlaintextToSecure is the end-to-end
// credibility test behind the README's encrypted-at-rest claim: a
// plaintext config passes through `dddns secure enable`, ends up as a
// 0400 .secure file, and LoadSecure returns the original field values.
func TestEnableSecure_RoundTripPlaintextToSecure(t *testing.T) {
	dir := t.TempDir()
	plaintextPath := filepath.Join(dir, "config.yaml")
	expectedSecure := filepath.Join(dir, "config.secure")

	const (
		wantAccessKey = "AKIAIOSFODNN7EXAMPLE"
		wantSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		wantZone      = "Z1ABCDEFGHIJKL"
		wantHost      = "test.example.com"
	)

	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"" + wantAccessKey + "\"\n" +
		"aws_secret_key: \"" + wantSecretKey + "\"\n" +
		"hosted_zone_id: \"" + wantZone + "\"\n" +
		"hostname: \"" + wantHost + "\"\n" +
		"ttl: 300\n" +
		"ip_cache_file: \"" + filepath.Join(dir, "last-ip.txt") + "\"\n"
	if err := os.WriteFile(plaintextPath, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}

	origCfgFile := cfgFile
	cfgFile = plaintextPath
	config.SetActivePath(plaintextPath)
	t.Cleanup(func() {
		cfgFile = origCfgFile
		config.SetActivePath("")
	})

	// Redirect stdout so the "Next steps" text doesn't clutter test output.
	_ = captureStdout(t, func() {
		if err := runEnableSecure(nil, nil); err != nil {
			t.Fatalf("runEnableSecure: %v", err)
		}
	})

	// Verify the secure file exists, has 0400 perms, and decrypts to the
	// original values.
	info, err := os.Stat(expectedSecure)
	if err != nil {
		t.Fatalf("secure config missing at %s: %v", expectedSecure, err)
	}
	if mode := info.Mode().Perm(); mode != constants.SecureConfigPerm {
		t.Errorf("secure config perms = %04o, want %04o", mode, constants.SecureConfigPerm)
	}
	// Plaintext should be wiped.
	if _, err := os.Stat(plaintextPath); !os.IsNotExist(err) {
		t.Errorf("plaintext config still present after migration: err=%v", err)
	}

	loaded, err := config.LoadSecure(expectedSecure)
	if err != nil {
		t.Fatalf("LoadSecure: %v", err)
	}
	if loaded.AWSAccessKey != wantAccessKey || loaded.AWSSecretKey != wantSecretKey {
		t.Error("credentials did not survive the migration round-trip")
	}
	if loaded.HostedZoneID != wantZone || loaded.Hostname != wantHost {
		t.Error("non-credential fields did not survive migration")
	}
}

// TestEnableSecure_RefusesWhenSecureAlreadyExists is the idempotence
// guard: running `dddns secure enable` twice must not silently clobber
// the existing .secure file (which could scramble a working vault).
func TestEnableSecure_RefusesWhenSecureAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	plaintextPath := filepath.Join(dir, "config.yaml")
	securePath := filepath.Join(dir, "config.secure")

	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"hostname: \"test.example.com\"\n" +
		"ttl: 300\n"
	if err := os.WriteFile(plaintextPath, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	// Pre-create the secure file as an empty placeholder.
	if err := os.WriteFile(securePath, []byte("# placeholder\n"), constants.SecureConfigPerm); err != nil {
		t.Fatalf("write placeholder secure: %v", err)
	}

	origCfgFile := cfgFile
	cfgFile = plaintextPath
	config.SetActivePath(plaintextPath)
	t.Cleanup(func() {
		cfgFile = origCfgFile
		config.SetActivePath("")
	})

	var runErr error
	_ = captureStdout(t, func() {
		runErr = runEnableSecure(nil, nil)
	})

	if runErr == nil {
		t.Fatal("runEnableSecure silently overwrote existing secure config")
	}
	if !strings.Contains(runErr.Error(), "already exists") {
		t.Errorf("error should cite existing secure config, got: %v", runErr)
	}
	// Plaintext must still exist — nothing was migrated.
	if _, err := os.Stat(plaintextPath); err != nil {
		t.Errorf("plaintext config lost despite aborted migration: %v", err)
	}
}

// TestTestSecure_EndToEndEncryptionRoundTrip exercises `dddns secure
// test`. It validates that device-key derivation, encryption, and
// decryption all work on this host — the same probe a user runs
// before migrating their real config.
func TestTestSecure_EndToEndEncryptionRoundTrip(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runTestSecure(nil, nil); err != nil {
			t.Fatalf("runTestSecure: %v", err)
		}
	})

	mustContainAll(t, out, []string{
		"Device key derived",
		"Test encryption successful",
		"Test decryption successful",
	})
}

func mustContainAll(t *testing.T, s string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Errorf("output missing %q\n---\n%s", n, s)
		}
	}
}
