package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"github.com/spf13/viper"
)

// Config holds all configuration for dddns
type Config struct {
	// AWS settings
	AWSRegion    string `mapstructure:"aws_region"`
	AWSAccessKey string `mapstructure:"aws_access_key"` // For standalone operation
	AWSSecretKey string `mapstructure:"aws_secret_key"` // For standalone operation

	// DNS settings (required)
	HostedZoneID string `mapstructure:"hosted_zone_id"`
	Hostname     string `mapstructure:"hostname"`
	TTL          int64  `mapstructure:"ttl"`

	// Operational settings
	IPCacheFile string `mapstructure:"ip_cache_file"`
	SkipProxy   bool   `mapstructure:"skip_proxy_check"`
	ForceUpdate bool   `mapstructure:"force_update"`
	DryRun      bool   `mapstructure:"dry_run"`
}

// Load reads configuration from file and environment
func Load() (*Config, error) {
	// Check if using secure config (from either viper or flag)
	configFile := viper.ConfigFileUsed()
	if configFile == "" && viper.IsSet("config") {
		configFile = viper.GetString("config")
	}
	if configFile != "" && strings.HasSuffix(configFile, ".secure") {
		// Load encrypted config
		return LoadSecure(configFile)
	}

	// Initialize profile system
	profile.Init()

	cfg := &Config{
		// Default values
		AWSRegion:   "us-east-1",
		TTL:         300,
		IPCacheFile: profile.Current.GetCachePath(),
		SkipProxy:   false,
		ForceUpdate: false,
		DryRun:      false,
	}

	// Load from viper (already initialized in cmd/root.go)
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Override with command-line flags if set
	if viper.IsSet("force") {
		cfg.ForceUpdate = viper.GetBool("force")
	}
	if viper.IsSet("dry-run") {
		cfg.DryRun = viper.GetBool("dry-run")
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// AWS credentials are required for security (no env vars allowed)
	if c.AWSAccessKey == "" {
		return fmt.Errorf("aws_access_key is required in config file")
	}
	if c.AWSSecretKey == "" {
		return fmt.Errorf("aws_secret_key is required in config file")
	}
	if c.HostedZoneID == "" {
		return fmt.Errorf("hosted_zone_id is required")
	}
	if c.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if c.TTL <= 0 {
		return fmt.Errorf("ttl must be positive")
	}
	return nil
}

// CreateDefault creates a default configuration file
func CreateDefault(path string) error {
	defaultConfig := `# dddns Configuration
# AWS Settings (REQUIRED - no env vars allowed for security)
aws_region: "us-east-1"  # AWS region
aws_access_key: ""       # REQUIRED: Your AWS Access Key
aws_secret_key: ""       # REQUIRED: Your AWS Secret Key

# DNS Settings (required)
hosted_zone_id: ""       # Your Route53 Hosted Zone ID
hostname: ""             # Domain name to update (e.g., home.example.com)
ttl: 300                 # TTL in seconds

# Operational Settings
ip_cache_file: "%s"  # Where to store last known IP
skip_proxy_check: false                   # Skip proxy/VPN detection
`

	// Create directory if needed
	dir := path[:len(path)-len("/config.yaml")]
	if err := os.MkdirAll(dir, constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Format template with proper cache path
	profile.Init()
	formattedConfig := fmt.Sprintf(defaultConfig, profile.Current.GetCachePath())

	// Write config file
	if err := os.WriteFile(path, []byte(formattedConfig), constants.ConfigFilePerm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
