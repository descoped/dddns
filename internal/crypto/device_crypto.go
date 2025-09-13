package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// GetDeviceKey derives a unique encryption key from device-specific data
func GetDeviceKey() ([]byte, error) {
	var deviceID string

	// Try UDM-specific identifiers first
	if data, err := os.ReadFile("/proc/ubnthal/system.info"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "serialno=") {
				deviceID = strings.TrimPrefix(line, "serialno=")
				break
			} else if strings.HasPrefix(line, "device.hashid=") {
				deviceID = strings.TrimPrefix(line, "device.hashid=")
				break
			}
		}
	}

	// Fallback to MAC address
	if deviceID == "" {
		if data, err := os.ReadFile("/sys/class/net/eth0/address"); err == nil {
			deviceID = strings.TrimSpace(string(data))
		}
	}

	// Last resort: hostname
	if deviceID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get device identifier: %w", err)
		}
		deviceID = hostname
	}

	// Add a salt for extra security
	salt := "dddns-vault-2025"
	combined := deviceID + salt

	// Derive 32-byte key using SHA256
	hash := sha256.Sum256([]byte(combined))
	return hash[:], nil
}

// EncryptCredentials encrypts AWS credentials using device-specific key
func EncryptCredentials(accessKey, secretKey string) (string, error) {
	key, err := GetDeviceKey()
	if err != nil {
		return "", err
	}

	// Combine credentials
	plaintext := fmt.Sprintf("%s:%s", accessKey, secretKey)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// GCM mode for authenticated encryption
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Return base64 encoded
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCredentials decrypts AWS credentials using device-specific key
func DecryptCredentials(encrypted string) (accessKey, secretKey string, err error) {
	key, err := GetDeviceKey()
	if err != nil {
		return "", "", err
	}

	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", "", err
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	// GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", "", err
	}

	// Split credentials
	parts := strings.SplitN(string(plaintext), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid credential format")
	}

	return parts[0], parts[1], nil
}

// SecureWipe overwrites sensitive data in memory
// Currently unused but kept for future security operations
//
//nolint:unused
func SecureWipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
