package crypto

import (
	"os"
	"strings"
	"testing"
)

// TestDeviceIDFallback_HostnameWithUser covers the USER-env branch.
// The function is the last resort when no platform-specific ID is
// available; a regression returning an empty or non-deterministic
// value would break the device-key derivation chain.
func TestDeviceIDFallback_HostnameWithUser(t *testing.T) {
	origUser, origUsername := os.Getenv("USER"), os.Getenv("USERNAME")
	t.Cleanup(func() {
		_ = os.Setenv("USER", origUser)
		_ = os.Setenv("USERNAME", origUsername)
	})

	if err := os.Setenv("USER", "testuser"); err != nil {
		t.Fatalf("Setenv USER: %v", err)
	}
	if err := os.Unsetenv("USERNAME"); err != nil {
		t.Fatalf("Unsetenv USERNAME: %v", err)
	}

	id, ok := deviceIDFallback()
	if !ok {
		t.Fatal("deviceIDFallback returned ok=false on a host with a resolvable hostname")
	}
	if !strings.HasSuffix(id, "-testuser") {
		t.Errorf("fallback id = %q, should end with -testuser", id)
	}
}

// TestDeviceIDFallback_UsernameBranch covers the Windows-style fallback
// where USERNAME is set but USER isn't (Go's os.Getenv is case-
// sensitive; the two variables have different provenance).
func TestDeviceIDFallback_UsernameBranch(t *testing.T) {
	origUser, origUsername := os.Getenv("USER"), os.Getenv("USERNAME")
	t.Cleanup(func() {
		_ = os.Setenv("USER", origUser)
		_ = os.Setenv("USERNAME", origUsername)
	})

	if err := os.Unsetenv("USER"); err != nil {
		t.Fatalf("Unsetenv USER: %v", err)
	}
	if err := os.Setenv("USERNAME", "winuser"); err != nil {
		t.Fatalf("Setenv USERNAME: %v", err)
	}

	id, ok := deviceIDFallback()
	if !ok {
		t.Fatal("deviceIDFallback returned ok=false")
	}
	if !strings.HasSuffix(id, "-winuser") {
		t.Errorf("fallback id = %q, should end with -winuser", id)
	}
}

// TestDeviceIDFallback_BareHostname covers the no-user branch — the
// function must still return a non-empty deterministic ID (just the
// hostname) rather than failing.
func TestDeviceIDFallback_BareHostname(t *testing.T) {
	origUser, origUsername := os.Getenv("USER"), os.Getenv("USERNAME")
	t.Cleanup(func() {
		_ = os.Setenv("USER", origUser)
		_ = os.Setenv("USERNAME", origUsername)
	})

	if err := os.Unsetenv("USER"); err != nil {
		t.Fatalf("Unsetenv USER: %v", err)
	}
	if err := os.Unsetenv("USERNAME"); err != nil {
		t.Fatalf("Unsetenv USERNAME: %v", err)
	}

	id, ok := deviceIDFallback()
	if !ok {
		t.Fatal("deviceIDFallback returned ok=false with resolvable hostname")
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Skipf("os.Hostname() failed on this platform: %v", err)
	}
	if id != hostname {
		t.Errorf("fallback id = %q, want bare hostname %q when neither USER nor USERNAME is set", id, hostname)
	}
}

// TestDeviceIDFallback_Deterministic guards the "derive the same key
// on every call" invariant. Non-determinism here would rotate the
// device key silently, making previously-encrypted secrets
// un-decryptable.
func TestDeviceIDFallback_Deterministic(t *testing.T) {
	a, okA := deviceIDFallback()
	b, okB := deviceIDFallback()
	if !okA || !okB {
		t.Fatal("deviceIDFallback ok=false")
	}
	if a != b {
		t.Errorf("deviceIDFallback not deterministic: %q vs %q", a, b)
	}
}
