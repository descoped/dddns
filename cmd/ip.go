package cmd

import (
	"fmt"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/spf13/cobra"
)

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Show current public IP address",
	Long:  `Display the current public IP address as seen from the internet.`,
	RunE:  runIP,
}

// init registers the ip command.
func init() {
	rootCmd.AddCommand(ipCmd)
}

// runIP retrieves and displays the current public IP address.
func runIP(cmd *cobra.Command, _ []string) error {
	ip, err := myip.GetPublicIP()
	if err != nil {
		return fmt.Errorf("failed to get public IP: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), ip)
	return nil
}
