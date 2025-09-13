// +build linux

package crypto_test

import (
	"os"
	"testing"

	"github.com/descoped/dddns/internal/crypto"
)

func TestGetDeviceKeyUDM(t *testing.T) {
	// This test runs in CI on Linux to simulate UDM environment
	t.Run("UDM Hardware ID", func(t *testing.T) {
		// Check if we're in simulated UDM environment (CI creates this)
		if _, err := os.Stat("/proc/ubnthal/system.info"); err == nil {
			key, err := crypto.GetDeviceKey()
			if err != nil {
				t.Fatalf("GetDeviceKey() failed in UDM environment: %v", err)
			}

			if len(key) != 32 {
				t.Errorf("Expected 32-byte key, got %d bytes", len(key))
			}

			// Verify key is deterministic
			key2, err := crypto.GetDeviceKey()
			if err != nil {
				t.Fatalf("Second GetDeviceKey() failed: %v", err)
			}

			for i := range key {
				if key[i] != key2[i] {
					t.Error("GetDeviceKey() is not deterministic")
					break
				}
			}

			t.Log("UDM hardware ID encryption key generated successfully")
		} else {
			t.Skip("Not in UDM environment")
		}
	})

	t.Run("Linux Fallback", func(t *testing.T) {
		// Test that GetDeviceKey works even without UDM hardware
		key, err := crypto.GetDeviceKey()
		if err != nil {
			t.Fatalf("GetDeviceKey() failed: %v", err)
		}

		if len(key) != 32 {
			t.Errorf("Expected 32-byte key, got %d bytes", len(key))
		}
	})

	t.Run("Docker Container ID", func(t *testing.T) {
		// Check if we're in Docker
		if _, err := os.Stat("/.dockerenv"); err == nil {
			key, err := crypto.GetDeviceKey()
			if err != nil {
				t.Fatalf("GetDeviceKey() failed in Docker: %v", err)
			}

			if len(key) != 32 {
				t.Errorf("Expected 32-byte key, got %d bytes", len(key))
			}

			t.Log("Docker container ID encryption key generated successfully")
		} else {
			t.Skip("Not in Docker environment")
		}
	})
}

func TestEncryptDecryptWithUDMKey(t *testing.T) {
	// Test encryption/decryption with platform-specific key
	accessKey := "AKIAIOSFODNN7EXAMPLE"
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	encrypted, err := crypto.EncryptCredentials(accessKey, secretKey)
	if err != nil {
		t.Fatalf("EncryptCredentials() failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("EncryptCredentials() returned empty string")
	}

	// Decrypt
	decryptedAccess, decryptedSecret, err := crypto.DecryptCredentials(encrypted)
	if err != nil {
		t.Fatalf("DecryptCredentials() failed: %v", err)
	}

	if decryptedAccess != accessKey {
		t.Errorf("Access key mismatch: got %q, want %q", decryptedAccess, accessKey)
	}

	if decryptedSecret != secretKey {
		t.Errorf("Secret key mismatch: got %q, want %q", decryptedSecret, secretKey)
	}
}