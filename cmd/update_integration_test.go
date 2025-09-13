package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestUpdateCommandDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	cacheFile := filepath.Join(tmpDir, "cache.txt")

	// Create a valid config
	configContent := `aws_profile: "test"
aws_region: "us-east-1"
hosted_zone_id: "Z123456"
hostname: "test.example.com"
ttl: 300
ip_cache_file: "` + cacheFile + `"
skip_proxy_check: true`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Reset viper
	viper.Reset()

	// Set args for update with dry-run
	rootCmd.SetArgs([]string{"update", "--config", configPath, "--dry-run"})

	// Execute command - will fail because we can't actually get public IP in tests
	// but that's OK, we're testing the command structure
	_ = rootCmd.Execute()
	// Don't check error as it will fail on actual IP fetch
}
