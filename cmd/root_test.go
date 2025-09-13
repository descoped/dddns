package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckConfigPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		perm    os.FileMode
		wantErr bool
	}{
		{"secure 600", 0600, false},
		{"secure 400", 0400, false},
		{"insecure 644", 0644, true},
		{"insecure 755", 0755, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(tmpDir, tt.name+".yaml")
			err := os.WriteFile(file, []byte("test"), tt.perm)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			err = checkConfigPermissions(file)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkConfigPermissions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInitConfig(t *testing.T) {
	// Save original config file
	origConfig := cfgFile
	defer func() { cfgFile = origConfig }()

	// Create test config first
	tmpDir := t.TempDir()
	cfgFile = filepath.Join(tmpDir, "config.yaml")

	configContent := `aws_region: us-west-2
aws_access_key: TEST_KEY
aws_secret_key: TEST_SECRET
hosted_zone_id: ZTEST
hostname: test.example.com
ttl: 300
skip_proxy_check: false
ip_cache_file: /tmp/test-cache.txt`

	err := os.WriteFile(cfgFile, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Test with existing config
	initConfig()
}
