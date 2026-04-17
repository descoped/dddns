package config

// activeConfigPath is set by cmd/root.go's initConfig once it has
// resolved which file to load. Load(), LoadSecure(), and commands that
// need to rewrite the current config (e.g. rotate-secret) read this
// value back via ActivePath().
var activeConfigPath string

// SetActivePath records the config path resolved by cmd/root.go's
// initConfig. Subsequent calls to Load()/LoadSecure() and ActivePath()
// read this value.
func SetActivePath(path string) { activeConfigPath = path }

// ActivePath returns the config file currently in use. Returns an
// empty string before initConfig has run (e.g. in isolated unit tests
// that bypass the cobra lifecycle).
func ActivePath() string { return activeConfigPath }
