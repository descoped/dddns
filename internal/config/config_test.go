package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/spf13/viper"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configContent := `aws_region: "eu-west-1"
aws_access_key: "AKIATEST"
aws_secret_key: "SECRETTEST"
hosted_zone_id: "Z1234567890ABC"
hostname: "test.example.com"
ttl: 600
ip_cache_file: "/tmp/test-dddns-cache.txt"`

	err := os.WriteFile(configFile, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Reset viper for clean test
	viper.Reset()
	viper.SetConfigFile(configFile)

	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded values
	// AWSProfile field has been removed from config
	if cfg.AWSRegion != "eu-west-1" {
		t.Errorf("Expected AWSRegion 'eu-west-1', got %q", cfg.AWSRegion)
	}
	if cfg.HostedZoneID != "Z1234567890ABC" {
		t.Errorf("Expected HostedZoneID 'Z1234567890ABC', got %q", cfg.HostedZoneID)
	}
	if cfg.Hostname != "test.example.com" {
		t.Errorf("Expected Hostname 'test.example.com', got %q", cfg.Hostname)
	}
	if cfg.TTL != 600 {
		t.Errorf("Expected TTL 600, got %d", cfg.TTL)
	}
	if cfg.IPCacheFile != "/tmp/test-dddns-cache.txt" {
		t.Errorf("Expected IPCacheFile '/tmp/test-dddns-cache.txt', got %q", cfg.IPCacheFile)
	}
}

func TestLoadConfigWithFlags(t *testing.T) {
	// Reset viper
	viper.Reset()

	// Set flags
	viper.Set("force", true)
	viper.Set("dry-run", true)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify flags override
	if !cfg.ForceUpdate {
		t.Error("Expected ForceUpdate to be true")
	}
	if !cfg.DryRun {
		t.Error("Expected DryRun to be true")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: config.Config{
				AWSAccessKey: "AKIATEST",
				AWSSecretKey: "SECRETTEST",
				HostedZoneID: "Z1234567890ABC",
				Hostname:     "test.example.com",
				TTL:          300,
			},
			wantErr: false,
		},
		{
			name: "missing hosted zone",
			config: config.Config{
				Hostname: "test.example.com",
				TTL:      300,
			},
			wantErr: true,
		},
		{
			name: "missing hostname",
			config: config.Config{
				HostedZoneID: "Z1234567890ABC",
				TTL:          300,
			},
			wantErr: true,
		},
		{
			name: "invalid TTL",
			config: config.Config{
				HostedZoneID: "Z1234567890ABC",
				Hostname:     "test.example.com",
				TTL:          0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".dddns", "config.yaml")

	err := config.CreateDefault(configPath)
	if err != nil {
		t.Fatalf("Failed to create default config: %v", err)
	}

	// Check file exists
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	// Check permissions
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Expected permissions 0600, got %04o", mode)
	}

	// Check content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Should contain required fields
	requiredFields := []string{
		"aws_region:",
		"aws_access_key:",
		"aws_secret_key:",
		"hosted_zone_id:",
		"hostname:",
		"ttl:",
		"ip_cache_file:",
	}

	for _, field := range requiredFields {
		if !contains(string(content), field) {
			t.Errorf("Config missing required field: %s", field)
		}
	}
}

func TestCreateDefaultConfig_InvalidPath(t *testing.T) {
	// Try to create config in a path that can't be created
	err := config.CreateDefault("/dev/null/config.yaml")
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

// TestLoadConfig_WithServerBlock verifies the new `server:` block parses
// correctly and round-trips through viper into a *ServerConfig on Config.
func TestLoadConfig_WithServerBlock(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `aws_region: "us-east-1"
aws_access_key: "AKIATEST"
aws_secret_key: "SECRETTEST"
hosted_zone_id: "Z1234567890ABC"
hostname: "test.example.com"
ttl: 300
ip_source: local
server:
  bind: "127.0.0.1:53353"
  shared_secret: "abc123"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "192.168.1.0/24"
  audit_log: "/var/log/dddns-audit.log"
  on_auth_failure: "logger"
  wan_interface: "eth8"`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	viper.Reset()
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.IPSource != "local" {
		t.Errorf("IPSource = %q, want local", cfg.IPSource)
	}
	if cfg.Server == nil {
		t.Fatal("Server block should be populated, got nil")
	}
	if cfg.Server.Bind != "127.0.0.1:53353" {
		t.Errorf("Bind = %q", cfg.Server.Bind)
	}
	if cfg.Server.SharedSecret != "abc123" {
		t.Errorf("SharedSecret not parsed")
	}
	if len(cfg.Server.AllowedCIDRs) != 2 {
		t.Errorf("AllowedCIDRs len = %d, want 2", len(cfg.Server.AllowedCIDRs))
	}
	if cfg.Server.WANInterface != "eth8" {
		t.Errorf("WANInterface = %q", cfg.Server.WANInterface)
	}
}

// TestLoadConfig_NoServerBlock verifies configs predating serve mode still
// load cleanly and leave Server nil / IPSource empty.
func TestLoadConfig_NoServerBlock(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `aws_region: "us-east-1"
aws_access_key: "AKIATEST"
aws_secret_key: "SECRETTEST"
hosted_zone_id: "Z1234567890ABC"
hostname: "test.example.com"
ttl: 300`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	viper.Reset()
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server != nil {
		t.Errorf("Server should be nil when block is absent, got %+v", cfg.Server)
	}
	if cfg.IPSource != "" {
		t.Errorf("IPSource should default to empty, got %q", cfg.IPSource)
	}
}

func TestConfigValidate_BadIPSource(t *testing.T) {
	cfg := config.Config{
		AWSAccessKey: "a",
		AWSSecretKey: "s",
		HostedZoneID: "Z",
		Hostname:     "h.example.com",
		TTL:          300,
		IPSource:     "bogus",
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ip_source") {
		t.Errorf("expected ip_source validation error, got: %v", err)
	}
}

func TestServerConfigValidate(t *testing.T) {
	good := config.ServerConfig{
		Bind:         "127.0.0.1:53353",
		SharedSecret: "secret",
		AllowedCIDRs: []string{"127.0.0.0/8", "192.168.1.0/24"},
	}
	if err := good.Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*config.ServerConfig)
		msg    string
	}{
		{"missing bind", func(s *config.ServerConfig) { s.Bind = "" }, "server.bind"},
		{"bad bind", func(s *config.ServerConfig) { s.Bind = "not-host-port" }, "host:port"},
		{"missing secret", func(s *config.ServerConfig) { s.SharedSecret = "" }, "shared_secret"},
		{"empty cidrs", func(s *config.ServerConfig) { s.AllowedCIDRs = nil }, "allowed_cidrs"},
		{"bad cidr", func(s *config.ServerConfig) { s.AllowedCIDRs = []string{"not-a-cidr"} }, "CIDR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := good
			s.AllowedCIDRs = append([]string{}, good.AllowedCIDRs...) // defensive copy
			tc.mutate(&s)
			err := s.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("error should mention %q, got: %v", tc.msg, err)
			}
		})
	}
}

// TestCreateDefaultConfig_NonStandardFilename verifies directory creation
// when the target filename is not exactly "config.yaml". The prior
// implementation stripped a hardcoded "/config.yaml" suffix from the path,
// which produced a malformed directory for any other filename (or panicked
// for paths shorter than 12 characters).
func TestCreateDefaultConfig_NonStandardFilename(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "custom.yaml")

	if err := config.CreateDefault(configPath); err != nil {
		t.Fatalf("CreateDefault failed on non-standard filename: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file not created at %s: %v", configPath, err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}
