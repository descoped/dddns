package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIVersion tests the version command
func TestCLIVersion(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(string(output), "dddns") {
		t.Errorf("version output missing 'dddns': %s", output)
	}
}

// TestCLIHelp tests the help command
func TestCLIHelp(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "root help",
			args:     []string{"--help"},
			expected: []string{"dddns updates AWS Route53", "Available Commands:", "config", "ip", "update", "verify"},
		},
		{
			name:     "config help",
			args:     []string{"config", "--help"},
			expected: []string{"Initialize and check configuration", "init", "check"},
		},
		{
			name:     "update help",
			args:     []string{"update", "--help"},
			expected: []string{"Check current public IP", "--dry-run", "--force"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"run", "../main.go"}, tc.args...)
			cmd := exec.Command("go", args...)
			output, _ := cmd.CombinedOutput()
			outputStr := string(output)

			for _, expected := range tc.expected {
				if !strings.Contains(outputStr, expected) {
					t.Errorf("help output missing %q\nGot: %s", expected, outputStr)
				}
			}
		})
	}
}

// TestConfigValidation tests config validation
func TestConfigValidation(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name        string
		config      string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: `aws_region: us-east-1
aws_access_key: AKIAIOSFODNN7EXAMPLE
aws_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
hosted_zone_id: Z1234567890ABC
hostname: test.example.com
ttl: 300`,
			shouldError: false,
		},
		{
			name: "missing credentials",
			config: `aws_region: us-east-1
hosted_zone_id: Z1234567890ABC
hostname: test.example.com
ttl: 300`,
			shouldError: true,
			errorMsg:    "aws_access_key is required",
		},
		{
			name: "missing hostname",
			config: `aws_region: us-east-1
aws_access_key: AKIAIOSFODNN7EXAMPLE
aws_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
hosted_zone_id: Z1234567890ABC
ttl: 300`,
			shouldError: true,
			errorMsg:    "hostname is required",
		},
		{
			name: "invalid TTL",
			config: `aws_region: us-east-1
aws_access_key: AKIAIOSFODNN7EXAMPLE
aws_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
hosted_zone_id: Z1234567890ABC
hostname: test.example.com
ttl: -1`,
			shouldError: true,
			errorMsg:    "ttl must be positive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tc.name+".yaml")
			if err := os.WriteFile(configPath, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("go", "run", "../main.go", "--config", configPath, "config", "check")
			output, err := cmd.CombinedOutput()

			if tc.shouldError {
				if err == nil {
					t.Errorf("expected error for %s, got none\nOutput: %s", tc.name, output)
				}
				if tc.errorMsg != "" && !strings.Contains(string(output), tc.errorMsg) {
					t.Errorf("expected error message %q, got: %s", tc.errorMsg, output)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v\nOutput: %s", tc.name, err, output)
				}
			}
		})
	}
}

// TestIPCommand tests the IP detection command
func TestIPCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping IP test in short mode (requires network)")
	}

	cmd := exec.Command("go", "run", "../main.go", "ip")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ip command failed: %v\nOutput: %s", err, output)
	}

	// Should return an IP-like string
	ip := strings.TrimSpace(string(output))
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		t.Errorf("expected IP address format, got: %s", ip)
	}
}

// TestDryRun tests the dry-run functionality
func TestDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	config := `aws_region: us-east-1
aws_access_key: AKIAIOSFODNN7EXAMPLE
aws_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
hosted_zone_id: Z1234567890ABC
hostname: test.example.com
ttl: 300
ip_cache_file: ` + filepath.Join(tmpDir, "cache.txt")

	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	// Test dry-run with custom IP
	cmd := exec.Command("go", "run", "../main.go", "--config", configPath, "update", "--dry-run", "--ip", "1.2.3.4")
	output, err := cmd.CombinedOutput()

	// Should succeed even with fake credentials in dry-run
	if err != nil && !strings.Contains(string(output), "DRY RUN") {
		t.Errorf("dry-run failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(string(output), "1.2.3.4") {
		t.Errorf("dry-run output should mention the IP address\nGot: %s", output)
	}

	// Cache file should NOT be created in dry-run
	cachePath := filepath.Join(tmpDir, "cache.txt")
	if _, err := os.Stat(cachePath); err == nil {
		t.Error("cache file should not be created in dry-run mode")
	}
}

// TestSecureCommand tests the secure encryption commands
func TestSecureCommand(t *testing.T) {
	// Test secure test command
	cmd := exec.Command("go", "run", "../main.go", "secure", "test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("secure test failed: %v\nOutput: %s", err, output)
	}

	expectedOutputs := []string{
		"Testing Device Encryption",
		"Device key derived",
		"Test encryption successful",
		"Test decryption successful",
		"Device profile:",
	}

	for _, expected := range expectedOutputs {
		if !strings.Contains(string(output), expected) {
			t.Errorf("secure test output missing %q\nGot: %s", expected, output)
		}
	}
}

// TestProfileDetection tests that profile detection works
func TestProfileDetection(t *testing.T) {
	cmd := exec.Command("go", "run", "../main.go", "secure", "test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("secure test failed: %v\nOutput: %s", err, output)
	}

	// Should detect the current OS profile
	validProfiles := []string{"macos", "linux", "udm", "docker"}
	foundProfile := false
	for _, profile := range validProfiles {
		if strings.Contains(string(output), "Device profile: "+profile) {
			foundProfile = true
			break
		}
	}

	if !foundProfile {
		t.Errorf("no valid profile detected in output: %s", output)
	}
}

// TestCachePersistence tests that IP cache works correctly
func TestCachePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test-cache.txt")

	// Write a cache file with old format (just IP)
	oldIP := "192.168.1.1"
	if err := os.WriteFile(cachePath, []byte(oldIP), 0600); err != nil {
		t.Fatal(err)
	}

	// Create config that uses this cache
	configPath := filepath.Join(tmpDir, "config.yaml")
	config := `aws_region: us-east-1
aws_access_key: AKIATEST
aws_secret_key: SECRETTEST
hosted_zone_id: Z123
hostname: test.example.com
ttl: 300
ip_cache_file: ` + cachePath

	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	// Run update in dry-run to see if it reads the cache
	cmd := exec.Command("go", "run", "../main.go", "--config", configPath, "update", "--dry-run", "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, _ := cmd.Output()

	// Should read the old IP from cache
	fullOutput := string(output) + stderr.String()
	if !strings.Contains(fullOutput, oldIP) {
		t.Errorf("should read cached IP %s\nOutput: %s", oldIP, fullOutput)
	}
}

// BenchmarkCLIStartup benchmarks how fast the CLI starts
func BenchmarkCLIStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("go", "run", "../main.go", "--help")
		_ = cmd.Run()
	}
}

// BenchmarkIPCommand benchmarks the IP command
func BenchmarkIPCommand(b *testing.B) {
	b.Skip("skipping network benchmark")

	for i := 0; i < b.N; i++ {
		cmd := exec.Command("go", "run", "../main.go", "ip")
		_ = cmd.Run()
	}
}