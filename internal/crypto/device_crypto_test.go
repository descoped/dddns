package crypto_test

import (
	"os"
	"testing"

	"github.com/descoped/dddns/internal/crypto"
)

func TestGetDeviceKey(t *testing.T) {
	// Test that device key generation is consistent
	key1, err := crypto.GetDeviceKey()
	if err != nil {
		t.Fatalf("Failed to get device key: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key1))
	}

	// Should produce same key on repeated calls
	key2, err := crypto.GetDeviceKey()
	if err != nil {
		t.Fatalf("Failed to get device key on second call: %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("Device key should be consistent across calls")
	}
}

func TestEncryptDecryptCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		accessKey string
		secretKey string
	}{
		{
			name:      "standard AWS credentials",
			accessKey: "AKIAIOSFODNN7EXAMPLE",
			secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name:      "short credentials",
			accessKey: "AK123",
			secretKey: "SK456",
		},
		{
			name:      "credentials with special characters",
			accessKey: "AKIA+TEST/KEY=",
			secretKey: "Secret+Key/With=Special@Chars!",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt credentials
			encrypted, err := crypto.EncryptCredentials(tc.accessKey, tc.secretKey)
			if err != nil {
				t.Fatalf("Failed to encrypt credentials: %v", err)
			}

			if encrypted == "" {
				t.Error("Encrypted data should not be empty")
			}

			// Encrypted data should be different from plaintext
			if encrypted == tc.accessKey || encrypted == tc.secretKey {
				t.Error("Encrypted data should not match plaintext")
			}

			// Decrypt credentials
			decAccess, decSecret, err := crypto.DecryptCredentials(encrypted)
			if err != nil {
				t.Fatalf("Failed to decrypt credentials: %v", err)
			}

			// Verify decrypted values match original
			if decAccess != tc.accessKey {
				t.Errorf("Access key mismatch: got %q, want %q", decAccess, tc.accessKey)
			}
			if decSecret != tc.secretKey {
				t.Errorf("Secret key mismatch: got %q, want %q", decSecret, tc.secretKey)
			}
		})
	}
}

func TestEncryptDecryptUniqueness(t *testing.T) {
	// Each encryption should produce different ciphertext due to random nonce
	accessKey := "AKIATEST"
	secretKey := "SECRETTEST"

	encrypted1, err := crypto.EncryptCredentials(accessKey, secretKey)
	if err != nil {
		t.Fatalf("First encryption failed: %v", err)
	}

	encrypted2, err := crypto.EncryptCredentials(accessKey, secretKey)
	if err != nil {
		t.Fatalf("Second encryption failed: %v", err)
	}

	// Different encryptions should produce different ciphertext
	if encrypted1 == encrypted2 {
		t.Error("Encrypted data should be different due to random nonce")
	}

	// But both should decrypt to same values
	dec1Access, dec1Secret, _ := crypto.DecryptCredentials(encrypted1)
	dec2Access, dec2Secret, _ := crypto.DecryptCredentials(encrypted2)

	if dec1Access != dec2Access || dec1Secret != dec2Secret {
		t.Error("Both encryptions should decrypt to same values")
	}
}

func TestDecryptInvalidData(t *testing.T) {
	testCases := []struct {
		name      string
		encrypted string
	}{
		{
			name:      "empty string",
			encrypted: "",
		},
		{
			name:      "invalid base64",
			encrypted: "not-valid-base64!@#$",
		},
		{
			name:      "valid base64 but not encrypted data",
			encrypted: "dGVzdCBkYXRh", // "test data" in base64
		},
		{
			name:      "corrupted encrypted data",
			encrypted: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := crypto.DecryptCredentials(tc.encrypted)
			if err == nil {
				t.Error("Expected error for invalid encrypted data")
			}
		})
	}
}

func TestSecureWipe(t *testing.T) {
	// Test that SecureWipe clears sensitive data
	sensitiveData := []byte("sensitive-password-12345")
	originalLen := len(sensitiveData)

	// Make a copy to verify original was modified
	dataCopy := make([]byte, len(sensitiveData))
	copy(dataCopy, sensitiveData)

	crypto.SecureWipe(sensitiveData)

	// Length should remain the same
	if len(sensitiveData) != originalLen {
		t.Error("SecureWipe should not change slice length")
	}

	// All bytes should be zero
	for i, b := range sensitiveData {
		if b != 0 {
			t.Errorf("Byte at position %d not wiped: %v", i, b)
		}
	}

	// Should not equal original data
	if string(sensitiveData) == string(dataCopy) {
		t.Error("SecureWipe failed to clear data")
	}
}

func TestDeviceKeyFallback(t *testing.T) {
	// Test that GetDeviceKey falls back to hostname when device ID not available
	// This test mainly ensures no panic/crash in fallback scenarios

	// Save original hostname
	originalHostname, _ := os.Hostname()

	// GetDeviceKey should work even without hardware ID files
	key, err := crypto.GetDeviceKey()
	if err != nil {
		// Only fail if we can't even get hostname
		if originalHostname == "" {
			t.Skip("Cannot test without hostname")
		}
		t.Fatalf("GetDeviceKey should fallback to hostname: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Expected 32-byte key from fallback, got %d bytes", len(key))
	}
}

func TestEncryptionWithDifferentKeys(t *testing.T) {
	// This test would require mocking the device key, which is not easily testable
	// in the current implementation. In production, different devices would have
	// different keys and encrypted data would not be portable.
	t.Skip("Cannot test different device keys without mocking")
}

func BenchmarkEncryptCredentials(b *testing.B) {
	accessKey := "AKIAIOSFODNN7EXAMPLE"
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := crypto.EncryptCredentials(accessKey, secretKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecryptCredentials(b *testing.B) {
	accessKey := "AKIAIOSFODNN7EXAMPLE"
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	encrypted, err := crypto.EncryptCredentials(accessKey, secretKey)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := crypto.DecryptCredentials(encrypted)
		if err != nil {
			b.Fatal(err)
		}
	}
}