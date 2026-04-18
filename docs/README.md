# dddns Documentation

Welcome to the dddns documentation! This guide will help you install, configure, and use dddns to automatically update your AWS Route53 DNS records with your dynamic IP address.

## What's New in v0.2.0

- **Serve mode (UniFi)** — event-driven updates via UniFi's built-in `inadyn` push, triggered the instant your WAN IP changes. Alternative to cron polling. See the [UDM Guide](udm-guide.md) and [Quick Start](quick-start.md).
- **Stdlib-only binary** — AWS SDK retired in favour of a hand-rolled Route53 REST client and SigV4 signer. Stripped ARM64 binary is ~7.8 MB (down from 16 MB); only direct dependencies are `cobra` and `yaml.v3`.
- **Hardened UniFi installer** — three safety gates (pre-flight, state snapshot, post-install smoke) plus `--probe`, `--version`, `--verbose`, `--rollback`, and `--uninstall`. See [Installation Guide](installation.md).
- **New commands** — `dddns serve` / `serve status` / `serve test`, `dddns config set-mode {cron|serve}`, `dddns config rotate-secret`. See [Command Reference](commands.md).
- **Stricter config permissions** — `config.yaml` is now refused at load time unless it's `chmod 600`. See [Configuration Guide](configuration.md).
- **WAN IP auto-detect fallback** — on devices with policy-based routing (e.g. UDR7), dddns now scans up interfaces when the main routing table has no default. See [Troubleshooting](troubleshooting.md).

## Documentation Structure

### Getting Started
- [**Quick Start**](quick-start.md) - Get up and running in 5 minutes
- [**AWS Setup Guide**](aws-setup.md) - Complete guide for setting up Route53 and IAM
- [**Installation Guide**](installation.md) - How to install dddns on various platforms
- [**Configuration Guide**](configuration.md) - Setting up AWS credentials and DNS settings

### Platform-Specific Guides
- [**Ubiquiti Dream Machine Guide**](udm-guide.md) - Complete guide for UDM/UDR devices

### Reference
- [**Command Reference**](commands.md) - All available commands and options
- [**Troubleshooting**](troubleshooting.md) - Common issues and solutions

## What is dddns?

dddns (Dynamic DNS) is a lightweight, efficient CLI tool that updates AWS Route53 DNS A records with your current public IP address. It's designed specifically for:

- 🏠 **Home networks** with dynamic IP addresses
- 🔒 **Ubiquiti Dream Machines** (UDM, UDR, UDR7, UDM-Pro, etc.)
- ⚡ **Resource-constrained devices** (< 20MB memory usage)
- 🔄 **Automated updates** via cron polling (recommended on UniFi Dream) or event-driven serve mode (same-host DDNS client)

## Key Features

- ✅ **Simple** - Single static binary (~7.8 MB stripped ARM64)
- ✅ **Stdlib-only** - Direct deps are `cobra` + `yaml.v3`; Route53 + SigV4 are hand-rolled, no AWS SDK
- ✅ **Two run modes** - Cron polling (works everywhere, recommended on UniFi) or event-driven serve mode (for same-host DDNS clients; see [UDM Guide](udm-guide.md) for the UniFi caveat)
- ✅ **Secure** - Encrypted credential storage with device-specific keys; 0600 config enforced at load
- ✅ **Efficient** - Minimal memory footprint, HTTP timeouts for reliability
- ✅ **Reliable** - IP change detection with persistent caching
- ✅ **Safe** - Proxy/VPN detection to prevent incorrect updates
- ✅ **Cron-friendly** - Quiet mode for unattended operation
- ✅ **Persistent** - Survives reboots and firmware updates on UniFi OS

## Available Commands

```bash
# Configuration management
dddns config init                 # Interactive configuration setup
dddns config check                # Validate configuration
dddns config set-mode cron|serve  # Switch UniFi run mode (rewrites boot script)
dddns config rotate-secret        # Rotate the serve-mode shared secret

# IP operations
dddns ip                          # Show current public IP address

# DNS updates
dddns update                      # Update DNS record if IP changed
dddns update --dry-run            # Preview what would be updated
dddns update --force              # Force update even if IP unchanged
dddns update --quiet              # Suppress non-error output (for cron)

# Verification
dddns verify                      # Check if DNS matches current IP

# Serve mode (UniFi event-driven bridge)
dddns serve                       # Start the listener (blocks; supervised by systemd)
dddns serve status                # Show the last request the listener handled
dddns serve test                  # Send a local Basic-Auth'd test request

# Security
dddns secure enable               # Convert to encrypted config
dddns secure test                 # Test device encryption
```

## Quick Example

```bash
# Initial setup
dddns config init

# Check your current public IP
dddns ip
# Output: 203.0.113.42

# Verify DNS status
dddns verify
# Output:
# Your public IP:     203.0.113.42
# Route53 record:     198.51.100.15
# ✗ DNS record doesn't match current IP

# Update DNS record (dry run first)
dddns update --dry-run
# Output: [DRY RUN] Would update home.example.com to 203.0.113.42

# Perform actual update
dddns update
# Output: Successfully updated home.example.com to 203.0.113.42

# Set up cron for automatic updates
crontab -e
# Add: */30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
```

## Security Features

### Encrypted Credentials
- Store AWS credentials encrypted with device-specific keys
- Credentials locked to specific hardware
- Secure memory wiping after use
- Read-only file permissions (0400)

### No Environment Variables
- Credentials must be in config file (no env vars for security)
- Prevents accidental exposure through process listings
- Config file permission enforcement

### Persistent Cache
- IP cache survives reboots on UDM (`/data/.dddns/last-ip.txt`)
- Prevents unnecessary API calls
- Timestamped for audit trail

## System Requirements

- **Architecture**: ARM64 (UDM/Pi) or AMD64 (standard Linux)
- **OS**: Linux, macOS
- **Memory**: < 20MB
- **Storage**: < 10MB binary + < 1KB config
- **Network**: Internet access to AWS Route53 and checkip.amazonaws.com
- **Permissions**: Read/write to config directory

## Platform Detection

dddns automatically detects your platform and adjusts paths accordingly:

| Platform | Config Path | Cache Path | Features |
|----------|------------|------------|----------|
| UDM/UDR | `/data/.dddns/config.yaml` | `/data/.dddns/last-ip.txt` | Persistent storage |
| Linux | `~/.dddns/config.yaml` | `~/.dddns/last-ip.txt` | Standard paths |
| macOS | `~/.dddns/config.yaml` | `~/.dddns/last-ip.txt` | Testing support |
| Docker | `/home/dddns/.dddns/config.yaml` | `/tmp/dddns-last-ip.txt` | Container support |

## Cron Best Practices

```bash
# Recommended cron entry with quiet mode and logging
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1

# With secure config on UDM
*/30 * * * * /data/dddns/dddns --config /data/.dddns/config.secure update --quiet >> /var/log/dddns.log 2>&1

# With log rotation (create /etc/logrotate.d/dddns)
/var/log/dddns.log {
    weekly
    rotate 4
    compress
    missingok
    notifempty
}
```

## Support

- **GitHub Issues**: [Report bugs or request features](https://github.com/descoped/dddns/issues)
- **Documentation**: Full guides in this docs/ directory

## License

dddns is open source software licensed under the MIT License.