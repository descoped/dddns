package cmd

import (
	"fmt"
	"strings"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/spf13/cobra"
)

// checkProxy flag determines whether to check if IP is from a proxy/VPN.
var checkProxy bool

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Show current public IP address",
	Long:  `Display the current public IP address as seen from the internet.`,
	RunE:  runIP,
}

// init registers the ip command and its flags.
func init() {
	rootCmd.AddCommand(ipCmd)
	ipCmd.Flags().BoolVar(&checkProxy, "check-proxy", false, "Check if IP is a proxy/VPN")
}

// runIP retrieves and displays the current public IP address.
// Optionally checks if the IP is from a proxy/VPN when --check-proxy flag is used.
func runIP(cmd *cobra.Command, _ []string) error {
	// Get public IP
	ip, err := myip.GetPublicIP()
	if err != nil {
		return fmt.Errorf("failed to get public IP: %w", err)
	}

	ip = strings.TrimSpace(ip)
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), ip)

	// Check proxy if requested
	if checkProxy {
		isProxy, err := myip.IsProxyIP(&ip)
		if err != nil {
			return fmt.Errorf("failed to check proxy status: %w", err)
		}

		if isProxy {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Status: Proxy/VPN detected")
		} else {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Status: Direct connection")
		}
	}

	return nil
}
