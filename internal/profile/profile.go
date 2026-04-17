package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/descoped/dddns/internal/constants"
)

// Profile defines deployment-specific configuration
type Profile struct {
	Name          string
	DataDir       string // Where to store config and cache
	ConfigPerm    os.FileMode
	SecurePerm    os.FileMode
	DirPerm       os.FileMode
	UseHardwareID bool   // Use device-specific encryption
	DeviceIDPath  string // Path to hardware identifier
}

var (
	// UDM profile for UniFi Dream Machine
	UDM = Profile{
		Name:          "udm",
		DataDir:       "/data/.dddns",
		ConfigPerm:    constants.ConfigFilePerm,
		SecurePerm:    constants.SecureConfigPerm,
		DirPerm:       constants.ConfigDirPerm,
		UseHardwareID: true,
		DeviceIDPath:  "/proc/ubnthal/system.info",
	}

	// Linux standard profile
	Linux = Profile{
		Name:          "linux",
		DataDir:       "$HOME/.dddns",
		ConfigPerm:    constants.ConfigFilePerm,
		SecurePerm:    constants.SecureConfigPerm,
		DirPerm:       constants.ConfigDirPerm,
		UseHardwareID: true, // Uses MAC address
		DeviceIDPath:  "/sys/class/net/eth0/address",
	}

	// MacOS profile
	MacOS = Profile{
		Name:          "macos",
		DataDir:       "$HOME/.dddns",
		ConfigPerm:    constants.ConfigFilePerm,
		SecurePerm:    constants.SecureConfigPerm,
		DirPerm:       constants.ConfigDirPerm,
		UseHardwareID: true,              // Uses hardware UUID or serial number
		DeviceIDPath:  "system_profiler", // Command-based retrieval
	}

	// Docker container profile
	Docker = Profile{
		Name:          "docker",
		DataDir:       "/config",
		ConfigPerm:    constants.ConfigFilePerm,
		SecurePerm:    constants.SecureConfigPerm,
		DirPerm:       constants.CacheDirPerm,
		UseHardwareID: true, // Uses container ID
		DeviceIDPath:  "/proc/self/cgroup",
	}

	// Windows profile (AMD64 and ARM64)
	Windows = Profile{
		Name:          "windows",
		DataDir:       "$APPDATA/dddns",
		ConfigPerm:    0600,
		SecurePerm:    0400,
		DirPerm:       0700,
		UseHardwareID: true,   // Uses machine GUID or hardware UUID
		DeviceIDPath:  "wmic", // Command-based retrieval
	}
)

// Detect identifies the deployment environment and returns the matching
// Profile. It is cheap (a few os.Stat calls + runtime.GOOS) — callers that
// need the profile repeatedly should cache the returned pointer locally
// instead of re-invoking.
func Detect() *Profile {
	// Check for UDM first (most specific)
	if _, err := os.Stat("/proc/ubnthal/system.info"); err == nil {
		return &UDM
	}

	// Check for Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return &Docker
	}

	// Check OS
	switch runtime.GOOS {
	case "darwin":
		return &MacOS
	case "linux":
		return &Linux
	case "windows":
		return &Windows
	default:
		return &Linux // Default fallback
	}
}

// GetDataDir returns the expanded data directory path. It fails loudly
// when $HOME/$APPDATA cannot be resolved — silently returning "/.dddns"
// would cause dddns to try to write under the filesystem root.
func (p *Profile) GetDataDir() (string, error) {
	switch p.DataDir {
	case "$HOME/.dddns":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		return filepath.Join(home, ".dddns"), nil
	case "$APPDATA/dddns":
		// Windows: Use %APPDATA% or fallback to user home
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "dddns"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		return filepath.Join(home, "AppData", "Roaming", "dddns"), nil
	default:
		return p.DataDir, nil
	}
}

// GetConfigPath returns the full config file path.
func (p *Profile) GetConfigPath() (string, error) {
	dir, err := p.GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// GetSecurePath returns the full secure config path.
func (p *Profile) GetSecurePath() (string, error) {
	dir, err := p.GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.secure"), nil
}

// GetCachePath returns the full cache file path.
func (p *Profile) GetCachePath() (string, error) {
	dir, err := p.GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last-ip.txt"), nil
}
