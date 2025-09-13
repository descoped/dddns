package profile

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestProfileDetection(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		skipOS   string
	}{
		{
			name:     "Docker Detection",
			expected: func() string {
				// Check if running in Docker
				if _, err := os.Stat("/.dockerenv"); err == nil {
					return "docker"
				}
				// Fall back to OS detection
				switch runtime.GOOS {
				case "linux":
					return "linux"
				case "darwin":
					return "macos"
				case "windows":
					return "windows"
				default:
					return "linux"
				}
			}(),
		},
		{
			name:     "macOS Detection",
			expected: "macos",
			skipOS:   "linux,windows",
		},
		{
			name:     "Windows Detection",
			expected: "windows",
			skipOS:   "linux,darwin",
		},
		{
			name:     "Linux Detection",
			expected: "linux",
			skipOS:   "darwin,windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOS != "" && strings.Contains(tt.skipOS, runtime.GOOS) {
				t.Skipf("Skipping %s test on %s", tt.name, runtime.GOOS)
			}

			profile := Detect()
			if tt.expected != "" && profile.Name != tt.expected {
				t.Errorf("Expected profile %s, got %s", tt.expected, profile.Name)
			}

			t.Logf("Detected profile: %s", profile.Name)
		})
	}
}

func TestAllProfilePaths(t *testing.T) {
	profiles := map[string]*Profile{
		"udm":     &UDM,
		"linux":   &Linux,
		"macos":   &MacOS,
		"docker":  &Docker,
		"windows": &Windows,
	}

	for name, profile := range profiles {
		t.Run(name, func(t *testing.T) {
			// Test that all path methods return non-empty strings
			dataDir := profile.GetDataDir()
			if dataDir == "" {
				t.Errorf("%s.GetDataDir() returned empty string", name)
			}

			configPath := profile.GetConfigPath()
			if configPath == "" {
				t.Errorf("%s.GetConfigPath() returned empty string", name)
			}
			if !strings.HasSuffix(configPath, "config.yaml") {
				t.Errorf("%s.GetConfigPath() = %v, should end with config.yaml", name, configPath)
			}

			securePath := profile.GetSecurePath()
			if securePath == "" {
				t.Errorf("%s.GetSecurePath() returned empty string", name)
			}
			if !strings.HasSuffix(securePath, "config.secure") {
				t.Errorf("%s.GetSecurePath() = %v, should end with config.secure", name, securePath)
			}

			cachePath := profile.GetCachePath()
			if cachePath == "" {
				t.Errorf("%s.GetCachePath() returned empty string", name)
			}
			if !strings.HasSuffix(cachePath, "last-ip.txt") {
				t.Errorf("%s.GetCachePath() = %v, should end with last-ip.txt", name, cachePath)
			}

			t.Logf("%s paths - Data: %s, Config: %s", name, dataDir, configPath)
		})
	}
}

func TestSecureConfigSupport(t *testing.T) {
	profiles := map[string]*Profile{
		"udm":     &UDM,
		"linux":   &Linux,
		"macos":   &MacOS,
		"docker":  &Docker,
		"windows": &Windows,
	}

	for name, profile := range profiles {
		t.Run(name+"_SecureConfig", func(t *testing.T) {
			if !profile.UseHardwareID {
				t.Errorf("%s should have UseHardwareID=true for secure config support", name)
			}

			// Verify DeviceIDPath is set (even if it's a command indicator)
			if profile.DeviceIDPath == "" && name != "udm" {
				t.Logf("%s uses command-based device ID retrieval", name)
			}
		})
	}
}

func TestProfileInit(t *testing.T) {
	// Test that Init() works without panicking
	Init()

	if Current == nil {
		t.Fatal("Init() should set Current profile")
	}

	t.Logf("Current profile after Init(): %s", Current.Name)

	// Test that calling Init() multiple times is safe
	firstCurrent := Current
	Init()

	if Current != firstCurrent {
		t.Error("Init() should not change Current if already set")
	}
}