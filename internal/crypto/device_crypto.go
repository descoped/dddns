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
	"os/exec"
	"runtime"
	"strings"
)

// GetDeviceKey derives a unique encryption key from device-specific data.
// It dispatches to a platform-specific collector, falling back to a
// hostname-based identity when none is available, then SHA-256s the
// result together with a project-wide salt.
func GetDeviceKey() ([]byte, error) {
	var deviceID string
	switch runtime.GOOS {
	case "linux":
		if id, ok := deviceIDLinux(); ok {
			deviceID = id
		}
	case "darwin":
		if id, ok := deviceIDDarwin(); ok {
			deviceID = id
		}
	case "windows":
		if id, ok := deviceIDWindows(); ok {
			deviceID = id
		}
	}
	if deviceID == "" {
		id, ok := deviceIDFallback()
		if !ok {
			return nil, fmt.Errorf("failed to get device identifier")
		}
		deviceID = id
	}

	const salt = "dddns-vault-2025"
	hash := sha256.Sum256([]byte(deviceID + salt))
	return hash[:], nil
}

// deviceIDLinux tries UDM hardware identifiers, then Docker container ID,
// then the eth0 MAC. Returns false if none are readable.
func deviceIDLinux() (string, bool) {
	// Try UDM-specific identifiers first.
	if data, err := os.ReadFile("/proc/ubnthal/system.info"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "serialno="); ok {
				return v, true
			}
			if v, ok := strings.CutPrefix(line, "device.hashid="); ok {
				return v, true
			}
		}
	}

	// Try Docker container ID.
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "docker") {
				continue
			}
			parts := strings.Split(line, "/")
			if len(parts) == 0 {
				continue
			}
			id := parts[len(parts)-1]
			if len(id) > 12 {
				id = id[:12]
			}
			return id, true
		}
	}

	// Fallback to MAC address.
	if data, err := os.ReadFile("/sys/class/net/eth0/address"); err == nil {
		mac := strings.TrimSpace(string(data))
		if mac != "" {
			return mac, true
		}
	}

	return "", false
}

// deviceIDDarwin reads the Hardware UUID, then the IOPlatformSerialNumber.
func deviceIDDarwin() (string, bool) {
	if out, err := exec.Command("system_profiler", "SPHardwareDataType").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "Hardware UUID:") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					return strings.TrimSpace(parts[1]), true
				}
			}
		}
	}

	if out, err := exec.Command("ioreg", "-l").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "IOPlatformSerialNumber") {
				parts := strings.Split(line, "=")
				if len(parts) > 1 {
					return strings.Trim(strings.TrimSpace(parts[1]), `"`), true
				}
			}
		}
	}

	return "", false
}

// deviceIDWindows reads the ComputerSystemProduct UUID via wmic, then
// falls back to the MachineGuid registry value.
func deviceIDWindows() (string, bool) {
	if out, err := exec.Command("wmic", "csproduct", "get", "UUID").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "UUID" || strings.Contains(line, "UUID") {
				continue
			}
			return line, true
		}
	}

	if out, err := exec.Command("cmd", "/c", "reg", "query", "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Cryptography", "/v", "MachineGuid").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, "MachineGuid") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) > 2 {
				return parts[len(parts)-1], true
			}
		}
	}

	return "", false
}

// deviceIDFallback returns hostname[-user] for uniqueness when no
// platform-specific ID was available. Returns false only if the hostname
// lookup itself fails.
func deviceIDFallback() (string, bool) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", false
	}
	if user := os.Getenv("USER"); user != "" {
		return hostname + "-" + user, true
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return hostname + "-" + user, true
	}
	return hostname, true
}

// EncryptString encrypts an arbitrary string using the device-specific key
// (AES-256-GCM, random nonce, base64-encoded result). The output includes
// the nonce as a prefix to the ciphertext.
func EncryptString(plaintext string) (string, error) {
	key, err := GetDeviceKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString reverses EncryptString. It fails if the ciphertext is
// corrupted, truncated, or was not produced on this device (the device
// key differs).
func DecryptString(encoded string) (string, error) {
	key, err := GetDeviceKey()
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, body := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// EncryptCredentials encrypts AWS credentials using the device-specific key.
// The access and secret keys are combined with a ":" separator.
func EncryptCredentials(accessKey, secretKey string) (string, error) {
	return EncryptString(accessKey + ":" + secretKey)
}

// DecryptCredentials reverses EncryptCredentials.
func DecryptCredentials(encrypted string) (accessKey, secretKey string, err error) {
	plaintext, err := DecryptString(encrypted)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(plaintext, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid credential format")
	}
	return parts[0], parts[1], nil
}
