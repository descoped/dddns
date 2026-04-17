package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rotateSecretInit  bool
	rotateSecretQuiet bool
)

var rotateSecretCmd = &cobra.Command{
	Use:   "rotate-secret",
	Short: "Regenerate the serve-mode shared secret and write it back to config",
	Long: `Generate a fresh 256-bit random shared secret and replace the
value in config. The config is rewritten in place — plaintext (.yaml)
or encrypted (.secure) is preserved.

The new secret is printed once. Copy it into the UniFi Network
Controller's Dynamic DNS "Password" field before the next IP change.

Use --init to create a default server block (loopback bind,
127.0.0.0/8 allowlist) if the config does not already have one — this
is how the installer seeds serve mode.`,
	RunE: runRotateSecret,
}

func init() {
	configCmd.AddCommand(rotateSecretCmd)
	rotateSecretCmd.Flags().BoolVar(&rotateSecretInit, "init", false, "Create a default server block if one does not exist")
	rotateSecretCmd.Flags().BoolVar(&rotateSecretQuiet, "quiet", false, "Print only the new secret on stdout (for scripting)")
}

// bindDefault is the fail-closed default for a freshly-initialised
// server block: loopback only, loopback-only CIDR allowlist.
const (
	defaultBind        = "127.0.0.1:53353"
	defaultAllowedCIDR = "127.0.0.0/8"
)

func runRotateSecret(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	path := configPath()
	if path == "" {
		return fmt.Errorf("could not determine config file path")
	}

	// Ensure there is a server block to update.
	if cfg.Server == nil {
		if !rotateSecretInit {
			return fmt.Errorf("no server block in config — pass --init to create one")
		}
		cfg.Server = &config.ServerConfig{
			Bind:         defaultBind,
			AllowedCIDRs: []string{defaultAllowedCIDR},
		}
	}

	// Generate the new secret.
	newSecret, err := generateSecret()
	if err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}
	cfg.Server.SharedSecret = newSecret

	// Write back, preserving the on-disk format.
	if strings.HasSuffix(path, ".secure") {
		if err := config.SaveSecure(cfg, path); err != nil {
			return fmt.Errorf("save secure config: %w", err)
		}
	} else {
		if err := config.SavePlaintext(cfg, path); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	// Best-effort audit entry. Failure here must not stop us from
	// printing the new secret.
	_ = server.NewAuditLog(server.AuditPath(cfg)).Write(server.AuditEntry{
		Action: "rotate-secret",
	})

	if rotateSecretQuiet {
		fmt.Fprintln(cmd.OutOrStdout(), newSecret)
	} else {
		printNewSecret(cmd.OutOrStdout(), newSecret, path)
	}
	return nil
}

// generateSecret returns a 256-bit random value as 64 lowercase hex chars.
func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// configPath returns the config file that was actually loaded — either
// the --config flag value or the path viper auto-discovered.
func configPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	return viper.ConfigFileUsed()
}

// printNewSecret writes the freshly-rotated secret in a clearly-framed
// block to out, along with UI-update instructions. This output must be
// distinctive — users need to notice and copy it.
func printNewSecret(out io.Writer, secret, path string) {
	bar := strings.Repeat("=", 65)
	fmt.Fprintln(out, bar)
	fmt.Fprintln(out, "  New shared secret generated ("+time.Now().UTC().Format(time.RFC3339)+")")
	fmt.Fprintln(out, bar)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+secret)
	fmt.Fprintln(out)
	fmt.Fprintln(out, bar)
	fmt.Fprintln(out, "  Written to: "+path)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Paste this value into the UniFi Network Controller's")
	fmt.Fprintln(out, "  Dynamic DNS Password field before the next IP change,")
	fmt.Fprintln(out, "  otherwise the next request will fail auth.")
	fmt.Fprintln(out, bar)
}
