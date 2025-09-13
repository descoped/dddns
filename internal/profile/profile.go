package profile

import (
	"os"
	"runtime"

	"github.com/descoped/dddns/internal/constants"
)

// Profile defines deployment-specific configuration
type Profile struct {
	Name         string
	DataDir      string // Where to store config and cache
	ConfigPerm   os.FileMode
	SecurePerm   os.FileMode
	DirPerm      os.FileMode
	UseHardwareID bool   // Use device-specific encryption
	DeviceIDPath string  // Path to hardware identifier
}

var (
	// UDM profile for UniFi Dream Machine
	UDM = Profile{
		Name:         "udm",
		DataDir:      "/data/.dddns",
		ConfigPerm:   constants.ConfigFilePerm,
		SecurePerm:   constants.SecureConfigPerm,
		DirPerm:      constants.ConfigDirPerm,
		UseHardwareID: true,
		DeviceIDPath: "/proc/ubnthal/system.info",
	}

	// Linux standard profile
	Linux = Profile{
		Name:         "linux",
		DataDir:      "$HOME/.dddns",
		ConfigPerm:   constants.ConfigFilePerm,
		SecurePerm:   constants.SecureConfigPerm,
		DirPerm:      constants.ConfigDirPerm,
		UseHardwareID: false,
		DeviceIDPath: "/sys/class/net/eth0/address",
	}

	// macOS profile
	MacOS = Profile{
		Name:         "macos",
		DataDir:      "$HOME/.dddns",
		ConfigPerm:   constants.ConfigFilePerm,
		SecurePerm:   constants.SecureConfigPerm,
		DirPerm:      constants.ConfigDirPerm,
		UseHardwareID: false,
		DeviceIDPath: "", // Use hostname only
	}

	// Docker container profile
	Docker = Profile{
		Name:         "docker",
		DataDir:      "/config",
		ConfigPerm:   constants.ConfigFilePerm,
		SecurePerm:   constants.SecureConfigPerm,
		DirPerm:      constants.CacheDirPerm,
		UseHardwareID: false,
		DeviceIDPath: "/proc/self/cgroup",
	}
)

// Current holds the active deployment profile
var Current *Profile

// Detect automatically detects the deployment environment
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
	default:
		return &Linux // Default fallback
	}
}

// Init initializes the profile system
func Init() {
	if Current == nil {
		Current = Detect()
	}
}

// GetDataDir returns the expanded data directory path
func (p *Profile) GetDataDir() string {
	if p.DataDir == "$HOME/.dddns" {
		home, _ := os.UserHomeDir()
		return home + "/.dddns"
	}
	return p.DataDir
}

// GetConfigPath returns the full config file path
func (p *Profile) GetConfigPath() string {
	return p.GetDataDir() + "/config.yaml"
}

// GetSecurePath returns the full secure config path
func (p *Profile) GetSecurePath() string {
	return p.GetDataDir() + "/config.secure"
}

// GetCachePath returns the full cache file path
func (p *Profile) GetCachePath() string {
	return p.GetDataDir() + "/last-ip.txt"
}