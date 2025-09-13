package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"github.com/descoped/dddns/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dddns/config.yaml)")
	_ = viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
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

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)

		// Handle .secure files specially
		if strings.HasSuffix(cfgFile, ".secure") {
			// For secure files, we just need to track the path
			// The actual loading will be handled by LoadSecure in config package
			viper.SetConfigType("yaml") // Set type to avoid "unsupported" error
		}
	} else {
		// Initialize profile system
		profile.Init()
		p := profile.Current

		// Check for secure config first (prefer encrypted over plaintext)
		securePath := p.GetSecurePath()
		if _, err := os.Stat(securePath); err == nil {
			// Found secure config, use it
			cfgFile = securePath
			viper.SetConfigFile(securePath)
			viper.SetConfigType("yaml")
		} else {
			// Fall back to regular config search
			viper.AddConfigPath(p.GetDataDir())
			viper.SetConfigName("config")
			viper.SetConfigType("yaml")
		}
	}

	// Read config file if it exists (skip for .secure files)
	if cfgFile != "" && strings.HasSuffix(cfgFile, ".secure") {
		// Don't try to read .secure files with viper
		// Just verify the file exists
		if _, err := os.Stat(cfgFile); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: config file not found: %v\n", err)
			os.Exit(1)
		}
	} else if err := viper.ReadInConfig(); err != nil {
		// Config file not found is okay for now, we'll use defaults
		var configNotFoundErr viper.ConfigFileNotFoundError
		if !errors.As(err, &configNotFoundErr) {
			_, _ = fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
		}
	}

	// Check config file permissions for security
	configFile := viper.ConfigFileUsed()
	if configFile == "" && cfgFile != "" {
		configFile = cfgFile // Use the flag value for .secure files
	}
	if configFile != "" {
		if err := checkConfigPermissions(configFile); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Security warning: %v\n", err)
			os.Exit(1)
		}
	}
}
