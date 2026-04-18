package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"go.yaml.in/yaml/v3"
)

// DefaultUpdateInterval is the crontab schedule written by `dddns config
// set-mode cron` when cfg.UpdateInterval is unset. 30 minutes is the same
// cadence UniFi's own inadyn uses by default.
const DefaultUpdateInterval = "*/30 * * * *"

// DefaultUpdateTimeout bounds a single `dddns update` run when
// cfg.UpdateTimeout is unset. 30 s is comfortable on a home connection;
// users on slower networks can raise it in config without recompiling.
const DefaultUpdateTimeout = 30 * time.Second

// Config holds all configuration for dddns.
type Config struct {
	// AWS settings
	AWSRegion    string `yaml:"aws_region"`
	AWSAccessKey string `yaml:"aws_access_key"`
	AWSSecretKey string `yaml:"aws_secret_key"`

	// DNS settings (required)
	HostedZoneID string `yaml:"hosted_zone_id"`
	Hostname     string `yaml:"hostname"`
	TTL          int64  `yaml:"ttl"`

	// Operational settings
	IPCacheFile string `yaml:"ip_cache_file"`

	// IPSource overrides where dddns obtains the current public IP.
	// Values: "" or "auto" (mode-driven default), "local" (read the WAN
	// interface), "remote" (call checkip.amazonaws.com). Serve mode always
	// reads the local interface regardless of this setting.
	IPSource string `yaml:"ip_source,omitempty"`

	// UpdateInterval overrides the crontab schedule written by
	// `dddns config set-mode cron`. Five-field crontab syntax.
	// Empty defaults to DefaultUpdateInterval ("*/30 * * * *").
	// Only consumed by the cron-mode bootscript generator; serve and
	// Lambda modes ignore it.
	UpdateInterval string `yaml:"update_interval,omitempty"`

	// UpdateTimeout bounds a single `dddns update` run's wall-clock
	// time. Go duration syntax ("30s", "2m", "500ms"). Empty defaults
	// to DefaultUpdateTimeout (30 s). Raise this if Route53 round-trips
	// on your network routinely approach 30 s.
	UpdateTimeout string `yaml:"update_timeout,omitempty"`

	// Server holds parameters for serve mode (dddns serve). nil when the
	// `server:` block is absent from the config file, which disables serve
	// mode. See ServerConfig for fields.
	Server *ServerConfig `yaml:"server,omitempty"`
}

// UpdateIntervalOrDefault returns cfg.UpdateInterval if set, otherwise
// DefaultUpdateInterval. Always returns a non-empty crontab schedule.
func (c *Config) UpdateIntervalOrDefault() string {
	if c.UpdateInterval != "" {
		return c.UpdateInterval
	}
	return DefaultUpdateInterval
}

// UpdateTimeoutOrDefault returns the parsed duration from
// cfg.UpdateTimeout, or DefaultUpdateTimeout when the field is unset or
// malformed. Malformed values are tolerated silently here — Validate()
// is responsible for surfacing parse errors at config-check time.
func (c *Config) UpdateTimeoutOrDefault() time.Duration {
	if c.UpdateTimeout == "" {
		return DefaultUpdateTimeout
	}
	d, err := time.ParseDuration(c.UpdateTimeout)
	if err != nil || d <= 0 {
		return DefaultUpdateTimeout
	}
	return d
}

// ServerConfig holds parameters for serve mode (dddns serve).
//
// The encrypted equivalent of SharedSecret lives in a sibling struct in
// secure_config.go (SecureServerConfig) so the two wire formats stay
// explicit.
type ServerConfig struct {
	Bind         string   `yaml:"bind"`
	SharedSecret string   `yaml:"shared_secret,omitempty"`
	AllowedCIDRs []string `yaml:"allowed_cidrs"`
	AuditLog     string   `yaml:"audit_log,omitempty"`
	WANInterface string   `yaml:"wan_interface,omitempty"`
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

// Load reads configuration from the file recorded by SetActivePath.
// Encrypted .secure paths are delegated to LoadSecure. Defaults are
// applied before YAML is parsed so any fields set in the file override
// them.
func Load() (*Config, error) {
	configFile := activeConfigPath
	if configFile != "" && strings.HasSuffix(configFile, ".secure") {
		return LoadSecure(configFile)
	}

	// Detect active profile for default path resolution.
	p := profile.Detect()
	cachePath, err := p.GetCachePath()
	if err != nil {
		return nil, fmt.Errorf("resolve cache path: %w", err)
	}

	cfg := &Config{
		// Default values — overridden by YAML below if present.
		AWSRegion:   "us-east-1",
		TTL:         300,
		IPCacheFile: cachePath,
	}

	// If no config file is active, return just defaults — the caller
	// will typically run Validate() which will report the missing
	// required fields.
	if configFile == "" {
		return cfg, nil
	}

	// Permission check BEFORE read: plaintext config holds AWS credentials,
	// so a world-readable file is a local-privilege-escalation vector on any
	// host where dddns runs alongside untrusted local accounts. Mirrors the
	// check LoadSecure performs on config.secure.
	info, err := os.Stat(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("stat config: %w", err)
	}
	if mode := info.Mode().Perm(); mode != constants.ConfigFilePerm {
		return nil, fmt.Errorf("config file %s has permissions %o, must be %o (chmod 600 %s)",
			configFile, mode, constants.ConfigFilePerm, configFile)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
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
	if c.UpdateTimeout != "" {
		d, err := time.ParseDuration(c.UpdateTimeout)
		if err != nil {
			return fmt.Errorf("update_timeout %q is not a valid duration (e.g. \"30s\", \"2m\"): %w", c.UpdateTimeout, err)
		}
		if d <= 0 {
			return fmt.Errorf("update_timeout %q must be positive", c.UpdateTimeout)
		}
	}
	// UpdateInterval has crontab syntax; full validation would pull in a
	// cron parser. Skip here — a malformed schedule surfaces immediately
	// when cron (re)loads the file on the target host, which is a faster
	// feedback loop than a parser in dddns would give.
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
