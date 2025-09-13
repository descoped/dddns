package config_test

import (
	"os"
	"path/filepath"
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
ip_cache_file: "/tmp/test-dddns-cache.txt"
skip_proxy_check: true`

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
	if !cfg.SkipProxy {
		t.Errorf("Expected SkipProxy true, got false")
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
		"skip_proxy_check:",
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}
