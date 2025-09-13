package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/crypto"
	"gopkg.in/yaml.v3"
)

// SecureConfig stores credentials in encrypted form
type SecureConfig struct {
	// AWS settings
	AWSRegion           string `yaml:"aws_region"`
	AWSCredentialsVault string `yaml:"aws_credentials_vault"` // Encrypted access:secret

	// DNS settings (not sensitive)
	HostedZoneID string `yaml:"hosted_zone_id"`
	Hostname     string `yaml:"hostname"`
	TTL          int64  `yaml:"ttl"`

	// Operational settings
	IPCacheFile string `yaml:"ip_cache_file"`
	SkipProxy   bool   `yaml:"skip_proxy_check"`
}

// SaveSecure saves config with encrypted credentials
func SaveSecure(cfg *Config, path string) error {
	// Encrypt credentials
	vault, err := crypto.EncryptCredentials(cfg.AWSAccessKey, cfg.AWSSecretKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// Create secure config
	secureCfg := &SecureConfig{
		AWSRegion:           cfg.AWSRegion,
		AWSCredentialsVault: vault,
		HostedZoneID:        cfg.HostedZoneID,
		Hostname:            cfg.Hostname,
		TTL:                 cfg.TTL,
		IPCacheFile:         cfg.IPCacheFile,
		SkipProxy:           cfg.SkipProxy,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(secureCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write with secure permissions (read-only)
	if err := os.WriteFile(path, data, constants.SecureConfigPerm); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// LoadSecure loads config with decrypted credentials
func LoadSecure(path string) (*Config, error) {
	// Check permissions
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	if mode != constants.ConfigFilePerm && mode != constants.SecureConfigPerm {
		return nil, fmt.Errorf("insecure permissions %04o (must be %04o or %04o)", mode, constants.ConfigFilePerm, constants.SecureConfigPerm)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Unmarshal YAML
	var secureCfg SecureConfig
	if err := yaml.Unmarshal(data, &secureCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Decrypt credentials
	accessKey, secretKey, err := crypto.DecryptCredentials(secureCfg.AWSCredentialsVault)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Return regular config
	return &Config{
		AWSRegion:    secureCfg.AWSRegion,
		AWSAccessKey: accessKey,
		AWSSecretKey: secretKey,
		HostedZoneID: secureCfg.HostedZoneID,
		Hostname:     secureCfg.Hostname,
		TTL:          secureCfg.TTL,
		IPCacheFile:  secureCfg.IPCacheFile,
		SkipProxy:    secureCfg.SkipProxy,
	}, nil
}

// MigrateToSecure converts plaintext config to encrypted
func MigrateToSecure(plaintextPath, securePath string) error {
	// Load plaintext config
	cfg, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load plaintext config: %w", err)
	}

	// Save as encrypted
	if err := SaveSecure(cfg, securePath); err != nil {
		return fmt.Errorf("failed to save secure config: %w", err)
	}

	// Securely wipe plaintext file
	info, _ := os.Stat(plaintextPath)
	if info != nil {
		// Overwrite with zeros
		zeros := make([]byte, info.Size())
		_ = os.WriteFile(plaintextPath, zeros, constants.ConfigFilePerm)
		// Remove file
		_ = os.Remove(plaintextPath)
	}

	fmt.Printf("✓ Migrated config to secure storage at %s\n", securePath)
	fmt.Println("✓ Original plaintext config has been securely wiped")

	return nil
}
