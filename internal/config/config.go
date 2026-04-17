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
	"gopkg.in/yaml.v3"
)

// Config holds all configuration for dddns.
type Config struct {
	// AWS settings
	AWSRegion    string `mapstructure:"aws_region"    yaml:"aws_region"`
	AWSAccessKey string `mapstructure:"aws_access_key" yaml:"aws_access_key"`
	AWSSecretKey string `mapstructure:"aws_secret_key" yaml:"aws_secret_key"`

	// DNS settings (required)
	HostedZoneID string `mapstructure:"hosted_zone_id" yaml:"hosted_zone_id"`
	Hostname     string `mapstructure:"hostname"       yaml:"hostname"`
	TTL          int64  `mapstructure:"ttl"            yaml:"ttl"`

	// Operational settings
	IPCacheFile string `mapstructure:"ip_cache_file" yaml:"ip_cache_file"`
	ForceUpdate bool   `mapstructure:"force_update"  yaml:"-"`
	DryRun      bool   `mapstructure:"dry_run"       yaml:"-"`

	// IPSource overrides where dddns obtains the current public IP.
	// Values: "" or "auto" (mode-driven default), "local" (read the WAN
	// interface), "remote" (call checkip.amazonaws.com). Serve mode always
	// reads the local interface regardless of this setting.
	IPSource string `mapstructure:"ip_source" yaml:"ip_source,omitempty"`

	// Server holds parameters for serve mode (dddns serve). nil when the
	// `server:` block is absent from the config file, which disables serve
	// mode. See ServerConfig for fields.
	Server *ServerConfig `mapstructure:"server" yaml:"server,omitempty"`
}

// ServerConfig holds parameters for serve mode (dddns serve).
//
// The same struct is used by the plaintext Config (via mapstructure/viper)
// and will be used by SecureConfig (via yaml.v3) — hence both tag sets.
// The encrypted equivalent of SharedSecret lives in a sibling struct in
// secure_config.go so the two wire formats stay explicit.
type ServerConfig struct {
	Bind         string   `mapstructure:"bind"           yaml:"bind"`
	SharedSecret string   `mapstructure:"shared_secret"  yaml:"shared_secret,omitempty"`
	AllowedCIDRs []string `mapstructure:"allowed_cidrs"  yaml:"allowed_cidrs"`
	AuditLog     string   `mapstructure:"audit_log"      yaml:"audit_log,omitempty"`
	WANInterface string   `mapstructure:"wan_interface"  yaml:"wan_interface,omitempty"`
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

	// Detect active profile for default path resolution.
	p := profile.Detect()
	cachePath, err := p.GetCachePath()
	if err != nil {
		return nil, fmt.Errorf("resolve cache path: %w", err)
	}

	cfg := &Config{
		// Default values
		AWSRegion:   "us-east-1",
		TTL:         300,
		IPCacheFile: cachePath,
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

// SavePlaintext serializes cfg to YAML and writes it to path with the
// standard plaintext permissions (0600). This rewrites the entire file;
// comments and formatting in any previous version are discarded.
//
// Use SaveSecure for encrypted-at-rest storage.
func SavePlaintext(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, data, constants.ConfigFilePerm); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// defaultConfigTemplate is the single source of truth for the commented
// YAML emitted by both `dddns config init` (non-interactive) and the
// interactive wizard. The %s placeholders (in order) are: AWS region,
// AWS access key, AWS secret key, hosted zone ID, hostname, TTL, and
// the IP cache file path.
const defaultConfigTemplate = `# dddns Configuration
# AWS Settings (REQUIRED - no env vars allowed for security)
aws_region: "%s"           # AWS region
aws_access_key: "%s"       # REQUIRED: Your AWS Access Key
aws_secret_key: "%s"       # REQUIRED: Your AWS Secret Key

# DNS Settings (required)
hosted_zone_id: "%s"       # Your Route53 Hosted Zone ID
hostname: "%s"             # Domain name to update (e.g., home.example.com)
ttl: %d                    # TTL in seconds

# Operational Settings
ip_cache_file: "%s"        # Where to store last known IP
`

// FormatConfigYAML renders cfg into the canonical commented YAML used by
// `dddns config init`. It never inspects or validates the config — callers
// are expected to call Config.Validate first when interactive input might
// have left required fields blank.
func FormatConfigYAML(cfg *Config) string {
	return fmt.Sprintf(
		defaultConfigTemplate,
		cfg.AWSRegion,
		cfg.AWSAccessKey,
		cfg.AWSSecretKey,
		cfg.HostedZoneID,
		cfg.Hostname,
		cfg.TTL,
		cfg.IPCacheFile,
	)
}

// CreateDefault creates a default configuration file.
func CreateDefault(path string) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	p := profile.Detect()
	cachePath, err := p.GetCachePath()
	if err != nil {
		return fmt.Errorf("resolve cache path: %w", err)
	}

	content := FormatConfigYAML(&Config{
		AWSRegion:   "us-east-1",
		TTL:         300,
		IPCacheFile: cachePath,
	})

	// Write config file
	if err := os.WriteFile(path, []byte(content), constants.ConfigFilePerm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
