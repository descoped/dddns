package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/crypto"
	"github.com/descoped/dddns/internal/profile"
	"github.com/spf13/cobra"
)

var secureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Secure credential management",
	Long:  `Enable device-encrypted credential storage for enhanced security.`,
}

var enableSecureCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable secure credential storage",
	Long:  `Convert plaintext config to device-encrypted storage using UDM hardware identifiers.`,
	RunE:  runEnableSecure,
}

var testSecureCmd = &cobra.Command{
	Use:   "test",
	Short: "Test device encryption",
	Long:  `Test that device-specific encryption is working correctly.`,
	RunE:  runTestSecure,
}

// init registers the secure command and its subcommands.
func init() {
	rootCmd.AddCommand(secureCmd)
	secureCmd.AddCommand(enableSecureCmd)
	secureCmd.AddCommand(testSecureCmd)
}

// runEnableSecure converts a plaintext config to encrypted storage.
// It uses device-specific encryption keys on supported platforms (UDM).
func runEnableSecure(_ *cobra.Command, _ []string) error {
	// Determine paths
	configPath := cfgFile
	if configPath == "" {
		// Use profile system for consistent path resolution
		profile.Init()
		configPath = profile.Current.GetConfigPath()
	}

	// Generate secure path
	var securePath string
	if strings.HasSuffix(configPath, ".yaml") {
		securePath = strings.TrimSuffix(configPath, ".yaml") + ".secure"
	} else {
		securePath = configPath + ".secure"
	}

	fmt.Println("=== Enable Secure Credential Storage ===")
	fmt.Println()
	fmt.Printf("Current config: %s\n", configPath)
	fmt.Printf("Secure config:  %s\n", securePath)
	fmt.Println()

	// Check if already using secure config
	if _, err := os.Stat(securePath); err == nil {
		return fmt.Errorf("secure config already exists at %s", securePath)
	}

	// Migrate to secure
	if err := config.MigrateToSecure(configPath, securePath); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Update your scripts to use: dddns --config " + securePath)
	fmt.Println("  2. Verify it works: dddns --config " + securePath + " verify")
	fmt.Println("  3. The original plaintext config has been securely wiped")

	return nil
}

// runTestSecure verifies that device-specific encryption is working.
// It performs encryption/decryption tests and displays device info.
func runTestSecure(_ *cobra.Command, _ []string) error {
	fmt.Println("=== Testing Device Encryption ===")
	fmt.Println()

	// Get device key
	key, err := crypto.GetDeviceKey()
	if err != nil {
		return fmt.Errorf("failed to get device key: %w", err)
	}

	fmt.Printf("✓ Device key derived: %x...\n", key[:8])

	// Test encryption/decryption
	testAccess := "AKIAIOSFODNN7EXAMPLE"
	testSecret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

	encrypted, err := crypto.EncryptCredentials(testAccess, testSecret)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	fmt.Printf("✓ Test encryption successful: %s...\n", encrypted[:32])

	decAccess, decSecret, err := crypto.DecryptCredentials(encrypted)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	if decAccess != testAccess || decSecret != testSecret {
		return fmt.Errorf("decryption mismatch")
	}

	fmt.Println("✓ Test decryption successful")
	fmt.Println()

	// Show device info sources
	profile.Init()
	p := profile.Current
	fmt.Printf("Device profile: %s\n", p.Name)
	fmt.Println("Device identifiers checked:")

	if p.DeviceIDPath != "" {
		if _, err := os.ReadFile(p.DeviceIDPath); err == nil {
			fmt.Printf("  ✓ %s\n", p.DeviceIDPath)
		} else {
			fmt.Printf("  ✗ %s (not found)\n", p.DeviceIDPath)
		}
	}

	if data, err := os.ReadFile("/sys/class/net/eth0/address"); err == nil {
		fmt.Printf("  ✓ MAC address: %s", data)
	}

	hostname, _ := os.Hostname()
	fmt.Printf("  ✓ Hostname: %s\n", hostname)

	fmt.Println()
	fmt.Println("Encryption is device-specific. Config files are NOT portable between devices.")

	return nil
}
