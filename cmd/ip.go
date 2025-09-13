package cmd

import (
	"fmt"
	"strings"

	"github.com/descoped/dddns/internal/commands/myip"
	"github.com/spf13/cobra"
)

var checkProxy bool

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Show current public IP address",
	Long:  `Display the current public IP address as seen from the internet.`,
	RunE:  runIP,
}

func init() {
	rootCmd.AddCommand(ipCmd)
	ipCmd.Flags().BoolVar(&checkProxy, "check-proxy", false, "Check if IP is a proxy/VPN")
}

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
