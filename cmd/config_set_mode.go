package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/descoped/dddns/internal/bootscript"
	"github.com/descoped/dddns/internal/config"
	"github.com/spf13/cobra"
)

// DefaultBootScriptPath is the UniFi location. Overridable via --boot-path
// for development, staging, or non-UniFi hosts.
const DefaultBootScriptPath = "/data/on_boot.d/20-dddns.sh"

var setModeBootPath string

var setModeCmd = &cobra.Command{
	Use:   "set-mode {cron|serve}",
	Short: "Switch between cron-driven updates and the serve-mode listener",
	Long: `Write the on_boot.d script for the chosen mode. The two modes
are mutually exclusive — serve mode starts a supervised dddns serve
loop; cron mode installs an /etc/cron.d entry that runs dddns update.

When switching to serve, the config must already contain a server
block. Use ` + "`dddns config rotate-secret --init`" + ` first to create one.

The generated script is idempotent — re-running it leaves the system
in the target state, switching away from the other mode as needed.`,
	Args: cobra.ExactArgs(1),
	RunE: runSetMode,
}

func init() {
	configCmd.AddCommand(setModeCmd)
	setModeCmd.Flags().StringVar(&setModeBootPath, "boot-path", DefaultBootScriptPath, "Destination path for the boot script")
}

func runSetMode(cmd *cobra.Command, args []string) error {
	mode := args[0]
	if mode != "cron" && mode != "serve" {
		return fmt.Errorf("mode must be 'cron' or 'serve', got %q", mode)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if mode == "serve" {
		if cfg.Server == nil {
			return fmt.Errorf("cannot switch to serve — config has no server block (run `dddns config rotate-secret --init` first)")
		}
		if err := cfg.Server.Validate(); err != nil {
			return fmt.Errorf("server block invalid: %w", err)
		}
	}

	script, err := bootscript.Generate(bootscript.DefaultUnifiParams(mode))
	if err != nil {
		return err
	}

	if err := writeBootScript(setModeBootPath, script); err != nil {
		return fmt.Errorf("write boot script: %w", err)
	}

	printSetModeInstructions(cmd.OutOrStdout(), mode, setModeBootPath)
	return nil
}

// writeBootScript writes body to path with 0755 perms, creating parent
// directories as needed.
func writeBootScript(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o755)
}

func printSetModeInstructions(out io.Writer, mode, path string) {
	fmt.Fprintf(out, "Wrote %s (mode=%s)\n", path, mode)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "To apply immediately, run as root:")
	fmt.Fprintf(out, "  sudo %s\n", path)
	fmt.Fprintln(out, "Or reboot the device — on_boot.d runs on every boot.")
}
