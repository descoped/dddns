package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/dns"
	"github.com/descoped/dddns/internal/profile"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
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
	// Determine config path.
	configPath := cfgFile
	if configPath == "" {
		p := profile.Detect()
		resolved, err := p.GetConfigPath()
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		configPath = resolved
	}

	// Check if file already exists.
	fileExists := false
	if _, err := os.Stat(configPath); err == nil {
		fileExists = true
		if !forceInit && !interactive {
			return fmt.Errorf("config file already exists at %s\nUse --force to overwrite or --interactive to update", configPath)
		}
	}

	// Interactive setup.
	if interactive {
		return runInteractiveConfig(configPath, fileExists)
	}

	// Non-interactive: create default config.
	if err := config.CreateDefault(configPath); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	fmt.Printf("Configuration file created at: %s\n", configPath)
	fmt.Println("Please edit this file with your AWS and DNS settings.")

	return nil
}

// maskKey masks sensitive keys for display.
func maskKey(key string) string {
	if key == "" {
		return "not set"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// readPrompt prints the prompt to stdout and returns the user's trimmed
// response from reader. On EOF / read error, the raw error is returned
// so callers can fail loud instead of silently continuing with "".
func readPrompt(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF after some bytes is fine — it's a terminal shutdown mid-line.
		// Only treat it as fatal when the line is completely empty.
		if err == io.EOF && line != "" {
			return strings.TrimSpace(line), nil
		}
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// promptConfig walks the user through the configuration prompts and
// returns a filled Config. It does not write anything to disk, and it
// does not validate — callers are expected to validate and confirm
// before saving.
func promptConfig(reader *bufio.Reader, existing *config.Config) (*config.Config, error) {
	exists := existing != nil
	cur := &config.Config{}
	if exists {
		cur = existing
	}

	fmt.Println("AWS Credentials (REQUIRED for security):")
	accessKey, err := readPrompt(reader, fmt.Sprintf("AWS Access Key ID [%s]: ", maskKey(cur.AWSAccessKey)))
	if err != nil {
		return nil, err
	}
	if accessKey == "" && exists {
		accessKey = cur.AWSAccessKey
	}

	secretKey, err := readPrompt(reader, fmt.Sprintf("AWS Secret Access Key [%s]: ", maskKey(cur.AWSSecretKey)))
	if err != nil {
		return nil, err
	}
	if secretKey == "" && exists {
		secretKey = cur.AWSSecretKey
	}

	defaultRegion := cur.AWSRegion
	if defaultRegion == "" {
		defaultRegion = "us-east-1"
	}
	region, err := readPrompt(reader, fmt.Sprintf("AWS Region [%s]: ", defaultRegion))
	if err != nil {
		return nil, err
	}
	if region == "" {
		region = defaultRegion
	}

	hostedZoneID, err := readPrompt(reader, fmt.Sprintf("Route53 Hosted Zone ID [%s]: ", cur.HostedZoneID))
	if err != nil {
		return nil, err
	}
	if hostedZoneID == "" && exists {
		hostedZoneID = cur.HostedZoneID
	}

	hostname, err := readPrompt(reader, fmt.Sprintf("Hostname to update (e.g., home.example.com) [%s]: ", cur.Hostname))
	if err != nil {
		return nil, err
	}
	if hostname == "" && exists {
		hostname = cur.Hostname
	}

	defaultTTL := cur.TTL
	if defaultTTL == 0 {
		defaultTTL = 300
	}
	ttlStr, err := readPrompt(reader, fmt.Sprintf("TTL in seconds [%d]: ", defaultTTL))
	if err != nil {
		return nil, err
	}
	ttl := defaultTTL
	if ttlStr != "" {
		_, _ = fmt.Sscanf(ttlStr, "%d", &ttl)
	}

	defaultCache := cur.IPCacheFile
	if defaultCache == "" {
		p := profile.Detect()
		cachePath, err := p.GetCachePath()
		if err != nil {
			return nil, fmt.Errorf("resolve cache path: %w", err)
		}
		defaultCache = cachePath
	}
	cacheFile, err := readPrompt(reader, fmt.Sprintf("IP cache file location [%s]: ", defaultCache))
	if err != nil {
		return nil, err
	}
	if cacheFile == "" {
		cacheFile = defaultCache
	}

	return &config.Config{
		AWSRegion:    region,
		AWSAccessKey: accessKey,
		AWSSecretKey: secretKey,
		HostedZoneID: hostedZoneID,
		Hostname:     hostname,
		TTL:          ttl,
		IPCacheFile:  cacheFile,
	}, nil
}

// summarizeConfig prints a human-readable summary of cfg to stdout, with
// credential fields masked.
func summarizeConfig(cfg *config.Config) {
	fmt.Println("=== Configuration Summary ===")
	fmt.Printf("AWS Access Key: %s\n", maskKey(cfg.AWSAccessKey))
	fmt.Printf("AWS Secret Key: %s\n", maskKey(cfg.AWSSecretKey))
	fmt.Printf("AWS Region: %s\n", cfg.AWSRegion)
	fmt.Printf("Hosted Zone ID: %s\n", cfg.HostedZoneID)
	fmt.Printf("Hostname: %s\n", cfg.Hostname)
	fmt.Printf("TTL: %d\n", cfg.TTL)
	fmt.Printf("Cache File: %s\n", cfg.IPCacheFile)
}

// runInteractiveConfig provides an interactive configuration wizard.
// It guides users through setting up AWS credentials and DNS settings,
// showing a summary and asking for confirmation before writing to disk.
func runInteractiveConfig(configPath string, exists bool) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== dddns Configuration Setup ===")
	fmt.Println()

	var existing *config.Config
	if exists {
		fmt.Printf("Found existing configuration at: %s\n", configPath)
		fmt.Println("Press Enter to keep current values, or type new ones.")
		fmt.Println()

		// Try to load existing config.
		if data, err := os.ReadFile(configPath); err == nil {
			var loaded config.Config
			if err := yaml.Unmarshal(data, &loaded); err == nil {
				existing = &loaded
			}
		}
	}

	cfg, err := promptConfig(reader, existing)
	if err != nil {
		return err
	}

	// Validate required fields before saving.
	if cfg.AWSAccessKey == "" || cfg.AWSSecretKey == "" {
		fmt.Println()
		fmt.Println("ERROR: AWS credentials are required for security.")
		fmt.Println("dddns does not use environment variables or IAM roles.")
		return fmt.Errorf("AWS credentials are required")
	}

	fmt.Println()
	summarizeConfig(cfg)
	fmt.Println()

	confirm, err := readPrompt(reader, "Save this configuration? (yes/no) [yes]: ")
	if err != nil {
		return err
	}
	confirm = strings.ToLower(confirm)
	if confirm == "" {
		confirm = "yes"
	}
	if confirm != "yes" && confirm != "y" {
		fmt.Println("Configuration cancelled.")
		return nil
	}

	// Create directory if needed.
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, constants.ConfigDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file using the single-source template.
	content := config.FormatConfigYAML(cfg)
	if err := os.WriteFile(configPath, []byte(content), constants.ConfigFilePerm); err != nil {
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
// It ensures all required settings are present and exercises the Route53
// credentials by looking up the configured hostname's current record.
func runConfigCheck(_ *cobra.Command, _ []string) error {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("✓ Configuration is valid")
	fmt.Printf("  AWS Region: %s\n", cfg.AWSRegion)
	fmt.Printf("  Hosted Zone ID: %s\n", cfg.HostedZoneID)
	fmt.Printf("  Hostname: %s\n", cfg.Hostname)
	fmt.Printf("  TTL: %d seconds\n", cfg.TTL)
	fmt.Printf("  Cache File: %s\n", cfg.IPCacheFile)

	// Test AWS credentials by attempting a Route53 lookup. We intentionally
	// do NOT fail the command on AWS errors — `config check` is a status
	// probe, not a gate. A broken AWS credential should be visible but
	// should not prevent the operator from continuing to diagnose.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r53, err := dns.NewFromConfig(ctx, cfg)
	if err != nil {
		fmt.Printf("  AWS credential check failed: %v\n", err)
		return nil
	}
	if _, err := r53.GetCurrentIP(ctx); err != nil {
		fmt.Printf("  AWS credential check failed: %v\n", err)
		return nil
	}
	fmt.Println("  AWS credentials verified (can list zone)")

	return nil
}
