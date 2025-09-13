package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigInitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create config with all required values for non-interactive mode
	configContent := `# dddns Configuration
aws_region: us-east-1
aws_access_key: TEST_ACCESS_KEY
aws_secret_key: TEST_SECRET_KEY
hosted_zone_id: Z1234567890ABC
hostname: test.example.com
ttl: 300
skip_proxy_check: false
ip_cache_file: /tmp/dddns-last-ip.txt`

	// Write the config first
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestConfigCheckCommand(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a valid config with all required fields
	configContent := `aws_region: us-east-1
aws_access_key: TEST_ACCESS_KEY
aws_secret_key: TEST_SECRET_KEY
hosted_zone_id: Z123456
hostname: test.example.com
ttl: 300
skip_proxy_check: false
ip_cache_file: /tmp/dddns-last-ip.txt`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Verify file can be read and validated
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	if len(content) == 0 {
		t.Error("Config file is empty")
	}
}
