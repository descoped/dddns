package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Configuration command flags.
var (
	forceInit   bool // forceInit overwrites existing configuration
	interactive bool // interactive enables interactive configuration setup
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  `Initialize and check configuration for dddns.`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create or update configuration file",
	Long:  `Create a new configuration file or interactively update an existing one.`,
	RunE:  runConfigInit,
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configuration",
	Long:  `Check if the configuration file is valid and AWS credentials are working.`,
	RunE:  runConfigCheck,
}

// init registers the config command and its subcommands.
func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(initCmd)
	configCmd.AddCommand(checkCmd)

	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "Overwrite existing configuration")
	initCmd.Flags().BoolVarP(&interactive, "interactive", "i", true, "Interactive configuration setup")
}

// runConfigInit creates or updates the configuration file.
// It supports both interactive and non-interactive modes.
func runConfigInit(_ *cobra.Command, _ []string) error {
	// Determine config path
	var configPath string
	if cfgFile != "" {
		configPath = cfgFile
	} else {
		// Use profile system for consistent path resolution
		profile.Init()
		configPath = profile.Current.GetConfigPath()
	}

	// Check if file already exists
	fileExists := false
	if _, err := os.Stat(configPath); err == nil {
		fileExists = true
		if !forceInit && !interactive {
			return fmt.Errorf("config file already exists at %s\nUse --force to overwrite or --interactive to update", configPath)
		}
	}

	// Interactive setup
	if interactive {
		return runInteractiveConfig(configPath, fileExists)
	}

	// Non-interactive: create default config
	if err := config.CreateDefault(configPath); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	fmt.Printf("Configuration file created at: %s\n", configPath)
	fmt.Println("Please edit this file with your AWS and DNS settings.")

	return nil
}

// maskKey masks sensitive keys for display
func maskKey(key string) string {
	if key == "" {
		return "not set"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// runInteractiveConfig provides an interactive configuration wizard.
// It guides users through setting up AWS credentials and DNS settings.
func runInteractiveConfig(configPath string, exists bool) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== dddns Configuration Setup ===")
	fmt.Println()

	// Load existing config if it exists
	var cfg config.Config
	if exists {
		fmt.Printf("Found existing configuration at: %s\n", configPath)
		fmt.Println("Press Enter to keep current values, or type new ones.")
		fmt.Println()

		// Try to load existing config
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err == nil {
			_ = viper.Unmarshal(&cfg)
		}
	}

	// AWS Credentials
	fmt.Println("AWS Credentials (REQUIRED for security):")
	fmt.Printf("AWS Access Key ID [%s]: ", maskKey(cfg.AWSAccessKey))
	awsAccessKey, _ := reader.ReadString('\n')
	awsAccessKey = strings.TrimSpace(awsAccessKey)
	if awsAccessKey == "" && exists {
		awsAccessKey = cfg.AWSAccessKey
	}

	fmt.Printf("AWS Secret Access Key [%s]: ", maskKey(cfg.AWSSecretKey))
	awsSecretKey, _ := reader.ReadString('\n')
	awsSecretKey = strings.TrimSpace(awsSecretKey)
	if awsSecretKey == "" && exists {
		awsSecretKey = cfg.AWSSecretKey
	}

	// AWS Region
	defaultRegion := cfg.AWSRegion
	if defaultRegion == "" {
		defaultRegion = "us-east-1"
	}
	fmt.Printf("AWS Region [%s]: ", defaultRegion)
	awsRegion, _ := reader.ReadString('\n')
	awsRegion = strings.TrimSpace(awsRegion)
	if awsRegion == "" {
		awsRegion = defaultRegion
	}

	// Hosted Zone ID
	fmt.Printf("Route53 Hosted Zone ID [%s]: ", cfg.HostedZoneID)
	hostedZoneID, _ := reader.ReadString('\n')
	hostedZoneID = strings.TrimSpace(hostedZoneID)
	if hostedZoneID == "" && exists {
		hostedZoneID = cfg.HostedZoneID
	}

	// Hostname
	fmt.Printf("Hostname to update (e.g., home.example.com) [%s]: ", cfg.Hostname)
	hostname, _ := reader.ReadString('\n')
	hostname = strings.TrimSpace(hostname)
	if hostname == "" && exists {
		hostname = cfg.Hostname
	}

	// TTL
	defaultTTL := cfg.TTL
	if defaultTTL == 0 {
		defaultTTL = 300
	}
	fmt.Printf("TTL in seconds [%d]: ", defaultTTL)
	ttlStr, _ := reader.ReadString('\n')
	ttlStr = strings.TrimSpace(ttlStr)
	ttl := defaultTTL
	if ttlStr != "" {
		_, _ = fmt.Sscanf(ttlStr, "%d", &ttl)
	}

	// Skip proxy check
	skipProxyDefault := "no"
	if cfg.SkipProxy {
		skipProxyDefault = "yes"
	}
	fmt.Printf("Skip proxy/VPN detection? (yes/no) [%s]: ", skipProxyDefault)
	skipProxyStr, _ := reader.ReadString('\n')
	skipProxyStr = strings.TrimSpace(strings.ToLower(skipProxyStr))
	if skipProxyStr == "" {
		skipProxyStr = skipProxyDefault
	}
	skipProxy := skipProxyStr == "yes" || skipProxyStr == "y"

	// Cache file location
	defaultCache := cfg.IPCacheFile
	if defaultCache == "" {
		// Use profile system for consistent cache path
		profile.Init()
		defaultCache = profile.Current.GetCachePath()
	}
	fmt.Printf("IP cache file location [%s]: ", defaultCache)
	cacheFile, _ := reader.ReadString('\n')
	cacheFile = strings.TrimSpace(cacheFile)
	if cacheFile == "" {
		cacheFile = defaultCache
	}

	// Create config content
	configContent := fmt.Sprintf(`# dddns Configuration
# AWS Settings (REQUIRED - no env vars allowed for security)
aws_region: "%s"           # AWS region
aws_access_key: "%s"       # REQUIRED: Your AWS Access Key
aws_secret_key: "%s"       # REQUIRED: Your AWS Secret Key

# DNS Settings (required)
hosted_zone_id: "%s"       # Your Route53 Hosted Zone ID
hostname: "%s"             # Domain name to update
ttl: %d                    # TTL in seconds

# Operational Settings
ip_cache_file: "%s"  # Where to store last known IP
skip_proxy_check: %t       # Skip proxy/VPN detection
`, awsRegion, awsAccessKey, awsSecretKey, hostedZoneID, hostname, ttl, cacheFile, skipProxy)

	// Validate required fields before saving
	if awsAccessKey == "" || awsSecretKey == "" {
		fmt.Println()
		fmt.Println("ERROR: AWS credentials are required for security.")
		fmt.Println("dddns does not use environment variables or IAM roles.")
		return fmt.Errorf("AWS credentials are required")
	}

	// Show summary
	fmt.Println()
	fmt.Println("=== Configuration Summary ===")
	fmt.Printf("AWS Access Key: %s\n", maskKey(awsAccessKey))
	fmt.Printf("AWS Secret Key: %s\n", maskKey(awsSecretKey))
	fmt.Printf("AWS Region: %s\n", awsRegion)
	fmt.Printf("Hosted Zone ID: %s\n", hostedZoneID)
	fmt.Printf("Hostname: %s\n", hostname)
	fmt.Printf("TTL: %d\n", ttl)
	fmt.Printf("Cache File: %s\n", cacheFile)
	fmt.Printf("Skip Proxy Check: %t\n", skipProxy)
	fmt.Println()

	// Confirm
	fmt.Print("Save this configuration? (yes/no) [yes]: ")
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm == "" {
		confirm = "yes"
	}

	if confirm != "yes" && confirm != "y" {
		fmt.Println("Configuration cancelled.")
		return nil
	}

	// Create directory if needed
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(configContent), constants.ConfigFilePerm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("\n✓ Configuration saved to: %s\n", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Test your configuration: dddns config check")
	fmt.Println("  2. Test IP detection: dddns ip")
	fmt.Println("  3. Do a dry run: dddns update --dry-run")
	fmt.Println("  4. Update DNS: dddns update")

	return nil
}

// runConfigCheck validates the configuration file and tests AWS connectivity.
// It ensures all required settings are present and credentials are working.
func runConfigCheck(_ *cobra.Command, _ []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("✓ Configuration is valid")
	fmt.Printf("  AWS Region: %s\n", cfg.AWSRegion)
	fmt.Printf("  Hosted Zone ID: %s\n", cfg.HostedZoneID)
	fmt.Printf("  Hostname: %s\n", cfg.Hostname)
	fmt.Printf("  TTL: %d seconds\n", cfg.TTL)
	fmt.Printf("  Cache File: %s\n", cfg.IPCacheFile)

	// TODO: Test AWS credentials by attempting to list hosted zones
	// This would require creating a Route53 client and making a test call

	return nil
}
