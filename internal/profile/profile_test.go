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
		setup    func()
		cleanup  func()
		expected string
		skipOS   string
	}{
		{
			name: "UDM Detection",
			setup: func() {
				// Simulate UDM environment
				if _, err := os.Stat("/proc/ubnthal/system.info"); err == nil {
					t.Log("Real UDM environment detected")
				}
			},
			cleanup: func() {},
			expected: func() string {
				if _, err := os.Stat("/proc/ubnthal/system.info"); err == nil {
					return "udm"
				}
				if runtime.GOOS == "linux" {
					return "linux"
				}
				return ""
			}(),
		},
		{
			name: "Docker Detection",
			setup: func() {
				// Docker detection would check /.dockerenv
				if _, err := os.Stat("/.dockerenv"); err == nil {
					t.Log("Docker environment detected")
				}
			},
			cleanup: func() {},
			expected: func() string {
				if _, err := os.Stat("/.dockerenv"); err == nil {
					return "docker"
				}
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
			setup:    func() {},
			cleanup:  func() {},
			expected: "macos",
			skipOS:   "linux,windows",
		},
		{
			name:     "Windows Detection",
			setup:    func() {},
			cleanup:  func() {},
			expected: "windows",
			skipOS:   "linux,darwin",
		},
		{
			name:     "Linux Detection",
			setup:    func() {},
			cleanup:  func() {},
			expected: "linux",
			skipOS:   "darwin,windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOS != "" && strings.Contains(tt.skipOS, runtime.GOOS) {
				t.Skipf("Skipping %s test on %s", tt.name, runtime.GOOS)
			}

			tt.setup()
			defer tt.cleanup()

			profile := Detect()
			if tt.expected != "" && profile.Name != tt.expected {
				t.Errorf("Expected profile %s, got %s", tt.expected, profile.Name)
			}
		})
	}
}

func TestProfilePaths(t *testing.T) {
	tests := []struct {
		name     string
		profile  *Profile
		wantData string
	}{
		{
			name:    "UDM Paths",
			profile: &UDM,
			wantData: "/data/.dddns",
		},
		{
			name:    "Docker Paths",
			profile: &Docker,
			wantData: "/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.GetDataDir(); got != tt.wantData {
				t.Errorf("GetDataDir() = %v, want %v", got, tt.wantData)
			}

			// Test config path construction
			configPath := tt.profile.GetConfigPath()
			if !strings.HasSuffix(configPath, "config.yaml") {
				t.Errorf("GetConfigPath() = %v, should end with config.yaml", configPath)
			}

			// Test secure path construction
			securePath := tt.profile.GetSecurePath()
			if !strings.HasSuffix(securePath, "config.secure") {
				t.Errorf("GetSecurePath() = %v, should end with config.secure", securePath)
			}

			// Test cache path construction
			cachePath := tt.profile.GetCachePath()
			if !strings.HasSuffix(cachePath, "last-ip.txt") {
				t.Errorf("GetCachePath() = %v, should end with last-ip.txt", cachePath)
			}
		})
	}
}

func TestConfigPaths(t *testing.T) {
	// Test that paths are correctly constructed for each profile
	profiles := []*Profile{&UDM, &Linux, &MacOS, &Docker, &Windows}

	for _, p := range profiles {
		t.Run(p.Name+"_Paths", func(t *testing.T) {
			// Skip Windows-specific test on non-Windows
			if p.Name == "windows" && runtime.GOOS != "windows" {
				// Just check path construction
				configPath := p.GetConfigPath()
				if configPath == "" {
					t.Error("GetConfigPath() returned empty string")
				}
				return
			}

			// Skip macOS-specific test on non-macOS
			if p.Name == "macos" && runtime.GOOS != "darwin" {
				// Just check path construction
				configPath := p.GetConfigPath()
				if configPath == "" {
					t.Error("GetConfigPath() returned empty string")
				}
				return
			}

			// All profiles should return non-empty paths
			if p.GetDataDir() == "" {
				t.Errorf("%s.GetDataDir() returned empty string", p.Name)
			}
			if p.GetConfigPath() == "" {
				t.Errorf("%s.GetConfigPath() returned empty string", p.Name)
			}
			if p.GetSecurePath() == "" {
				t.Errorf("%s.GetSecurePath() returned empty string", p.Name)
			}
			if p.GetCachePath() == "" {
				t.Errorf("%s.GetCachePath() returned empty string", p.Name)
			}
		})
	}
}

func TestHardwareIDSupport(t *testing.T) {
	profiles := map[string]*Profile{
		"udm":     &UDM,
		"linux":   &Linux,
		"macos":   &MacOS,
		"docker":  &Docker,
		"windows": &Windows,
	}

	for name, p := range profiles {
		t.Run(name+"_HardwareID", func(t *testing.T) {
			if !p.UseHardwareID {
				t.Errorf("%s should have UseHardwareID=true for secure config support", name)
			}
		})
	}
}