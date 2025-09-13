package version

import (
	_ "embed"
	"strings"
)

// Version is set at build time via ldflags, or embedded from VERSION file
var Version = "dev"

// BuildDate is set at build time via ldflags
var BuildDate = "unknown"

//go:embed VERSION
var versionFile string

func init() {
	// If Version wasn't set via ldflags, use the embedded VERSION file
	if Version == "dev" || Version == "" {
		Version = strings.TrimSpace(versionFile)
	}
}

// GetVersion returns the current version
func GetVersion() string {
	if Version == "" {
		return "dev"
	}
	return Version
}

// GetBuildDate returns the build date
func GetBuildDate() string {
	return BuildDate
}

// GetFullVersion returns version with build date
func GetFullVersion() string {
	v := GetVersion()
	if BuildDate != "" && BuildDate != "unknown" {
		return v + " (" + BuildDate + ")"
	}
	return v
}