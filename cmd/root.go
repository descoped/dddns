package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"github.com/descoped/dddns/internal/version"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "dddns",
		Short: "Dynamic DNS updater for AWS Route53",
		Long: `dddns updates AWS Route53 DNS A records with your current public IP address.
Designed to run via cron on Ubiquiti Dream Machine routers.`,
		Version: version.GetFullVersion(),
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

// init initializes the root command with global flags and configuration.
func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dddns/config.yaml)")
}

// checkConfigPermissions ensures config file has secure permissions (600 or 400)
func checkConfigPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	// Accept only standard or secure config permissions
	if mode != constants.ConfigFilePerm && mode != constants.SecureConfigPerm {
		return fmt.Errorf("config file %s has insecure permissions %04o (must be %04o or %04o)", path, mode, constants.ConfigFilePerm, constants.SecureConfigPerm)
	}

	return nil
}

// fatalf writes a formatted error message to stderr and exits non-zero.
// Used during config initialization (cobra.OnInitialize) where cobra
// has not yet taken over execution and has no error-return hook we can
// wire into — os.Exit is the only way to surface the failure before
// a command's RunE runs.
func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// initConfig resolves the config file path and records it with
// config.SetActivePath so config.Load can pick it up later.
//
// Resolution priority:
//  1. --config flag (any extension, used verbatim).
//  2. <profile data dir>/config.secure (preferred when present).
//  3. <profile data dir>/config.yaml.
//
// A missing file is not fatal here: the user may be about to run
// `dddns config init`. Any other stat error (permissions, I/O) IS
// fatal — silently continuing would surface downstream as a confusing
// "aws_access_key is required" error that hides the real cause.
func initConfig() {
	resolved := cfgFile
	if resolved == "" {
		p := profile.Detect()

		securePath, err := p.GetSecurePath()
		if err != nil {
			fatalf("Error resolving secure config path: %v", err)
		}
		if _, err := os.Stat(securePath); err == nil {
			resolved = securePath
		} else if !errors.Is(err, os.ErrNotExist) {
			fatalf("Error reading config file: %v", err)
		} else {
			dataDir, err := p.GetDataDir()
			if err != nil {
				fatalf("Error resolving data directory: %v", err)
			}
			yamlPath := filepath.Join(dataDir, "config.yaml")
			if _, err := os.Stat(yamlPath); err == nil {
				resolved = yamlPath
			} else if !errors.Is(err, os.ErrNotExist) {
				fatalf("Error reading config file: %v", err)
			}
			// If yamlPath also missing, resolved stays "" — Load()
			// will return defaults and Validate() will surface the
			// real problem when a command actually needs config.
		}
	}

	// Flag-supplied path must exist (stat fatal on any error).
	if cfgFile != "" {
		if _, err := os.Stat(cfgFile); err != nil {
			fatalf("Error: config file not found: %v", err)
		}
	}

	cfgFile = resolved
	config.SetActivePath(resolved)

	if resolved != "" {
		if err := checkConfigPermissions(resolved); err != nil {
			fatalf("Security warning: %v", err)
		}
		// Eagerly parse plaintext YAML so a malformed file fails
		// fast with a clear "Error reading config file" message
		// instead of surfacing later as a validation error.
		// .secure files are binary-ish and get parsed by LoadSecure
		// during config.Load.
		if !strings.HasSuffix(resolved, ".secure") {
			data, err := os.ReadFile(resolved)
			if err != nil {
				fatalf("Error reading config file: %v", err)
			}
			var probe map[string]interface{}
			if err := yaml.Unmarshal(data, &probe); err != nil {
				fatalf("Error reading config file: %v", err)
			}
		}
	}
}
