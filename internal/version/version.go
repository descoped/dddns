package version

// Version is set at build time via ldflags
var Version = "dev"

// BuildDate is set at build time via ldflags
var BuildDate = "unknown"

// Commit is set at build time via ldflags
var Commit = "none"

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