package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"github.com/spf13/viper"
)

// Config holds all configuration for dddns.
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
	ForceUpdate bool   `mapstructure:"force_update"`
	DryRun      bool   `mapstructure:"dry_run"`

	// IPSource overrides where dddns obtains the current public IP.
	// Values: "" or "auto" (mode-driven default), "local" (read the WAN
	// interface), "remote" (call checkip.amazonaws.com). Serve mode always
	// reads the local interface regardless of this setting.
	IPSource string `mapstructure:"ip_source"`

	// Server holds parameters for serve mode (dddns serve). nil when the
	// `server:` block is absent from the config file, which disables serve
	// mode. See ServerConfig for fields.
	Server *ServerConfig `mapstructure:"server"`
}

// ServerConfig holds parameters for serve mode (dddns serve).
//
// The same struct is used by the plaintext Config (via mapstructure/viper)
// and will be used by SecureConfig (via yaml.v3) — hence both tag sets.
// The encrypted equivalent of SharedSecret lives in a sibling struct in
// secure_config.go so the two wire formats stay explicit.
type ServerConfig struct {
	Bind          string   `mapstructure:"bind"           yaml:"bind"`
	SharedSecret  string   `mapstructure:"shared_secret"  yaml:"shared_secret,omitempty"`
	AllowedCIDRs  []string `mapstructure:"allowed_cidrs"  yaml:"allowed_cidrs"`
	AuditLog      string   `mapstructure:"audit_log"      yaml:"audit_log,omitempty"`
	OnAuthFailure string   `mapstructure:"on_auth_failure" yaml:"on_auth_failure,omitempty"`
	WANInterface  string   `mapstructure:"wan_interface"  yaml:"wan_interface,omitempty"`
}

// Validate reports whether the server block is well-formed. It is called
// by `dddns serve` before binding, and by `dddns config set-mode serve`
// before rewriting the boot script. The cron path does not need to call
// this — Config.Validate ignores the server block when the user only
// runs `dddns update`.
func (s *ServerConfig) Validate() error {
	if s.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	if _, _, err := net.SplitHostPort(s.Bind); err != nil {
		return fmt.Errorf("server.bind %q is not host:port: %w", s.Bind, err)
	}
	if s.SharedSecret == "" {
		return fmt.Errorf("server.shared_secret is required (or server.secret_vault in secure config)")
	}
	if len(s.AllowedCIDRs) == 0 {
		return fmt.Errorf("server.allowed_cidrs must be non-empty (fail-closed)")
	}
	for _, c := range s.AllowedCIDRs {
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("server.allowed_cidrs: %q is not a valid CIDR: %w", c, err)
		}
	}
	return nil
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

// Validate checks the top-level Config. It does not validate the Server
// block — that is ServerConfig.Validate's job, called by `dddns serve`.
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
	switch c.IPSource {
	case "", "auto", "local", "remote":
		// ok
	default:
		return fmt.Errorf("ip_source %q must be one of: auto, local, remote", c.IPSource)
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
`

	// Create directory if needed
	dir := filepath.Dir(path)
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
