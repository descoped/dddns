package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/crypto"
	"gopkg.in/yaml.v3"
)

// SecureConfig stores credentials in encrypted form.
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
	IPSource    string `yaml:"ip_source,omitempty"`

	// Server holds the serve-mode parameters. SecretVault is the encrypted
	// form of the plaintext ServerConfig.SharedSecret.
	Server *SecureServerConfig `yaml:"server,omitempty"`
}

// SecureServerConfig is the at-rest form of ServerConfig with the shared
// secret replaced by a device-encrypted vault.
type SecureServerConfig struct {
	Bind          string   `yaml:"bind"`
	SecretVault   string   `yaml:"secret_vault"`
	AllowedCIDRs  []string `yaml:"allowed_cidrs"`
	AuditLog      string   `yaml:"audit_log,omitempty"`
	OnAuthFailure string   `yaml:"on_auth_failure,omitempty"`
	WANInterface  string   `yaml:"wan_interface,omitempty"`
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
		IPSource:            cfg.IPSource,
	}

	// Encrypt the server block if present.
	if cfg.Server != nil {
		secretVault, err := crypto.EncryptString(cfg.Server.SharedSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt server.shared_secret: %w", err)
		}
		secureCfg.Server = &SecureServerConfig{
			Bind:          cfg.Server.Bind,
			SecretVault:   secretVault,
			AllowedCIDRs:  cfg.Server.AllowedCIDRs,
			AuditLog:      cfg.Server.AuditLog,
			OnAuthFailure: cfg.Server.OnAuthFailure,
			WANInterface:  cfg.Server.WANInterface,
		}
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

	// When re-writing over an existing .secure file, its 0400 perms
	// prevent os.WriteFile from truncating it. Chmod back to owner-
	// writable first; the final chmod below restores 0400.
	if info, err := os.Stat(path); err == nil && info.Mode().Perm() == constants.SecureConfigPerm {
		if err := os.Chmod(path, constants.ConfigFilePerm); err != nil {
			return fmt.Errorf("chmod secure config for rewrite: %w", err)
		}
	}

	if err := os.WriteFile(path, data, constants.SecureConfigPerm); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	// Ensure the final perm is 0400 even if the file pre-existed at 0600.
	if err := os.Chmod(path, constants.SecureConfigPerm); err != nil {
		return fmt.Errorf("chmod secure config: %w", err)
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

	// Decrypt the server block if present.
	var serverCfg *ServerConfig
	if secureCfg.Server != nil {
		sharedSecret, err := crypto.DecryptString(secureCfg.Server.SecretVault)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt server.secret_vault: %w", err)
		}
		serverCfg = &ServerConfig{
			Bind:          secureCfg.Server.Bind,
			SharedSecret:  sharedSecret,
			AllowedCIDRs:  secureCfg.Server.AllowedCIDRs,
			AuditLog:      secureCfg.Server.AuditLog,
			OnAuthFailure: secureCfg.Server.OnAuthFailure,
			WANInterface:  secureCfg.Server.WANInterface,
		}
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
		IPSource:     secureCfg.IPSource,
		Server:       serverCfg,
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
