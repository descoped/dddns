# dddns Documentation

Welcome to the dddns documentation! This guide will help you install, configure, and use dddns to automatically update your AWS Route53 DNS records with your dynamic IP address.

## 📚 Documentation Structure

### Getting Started
- [**Installation Guide**](INSTALLATION.md) - How to install dddns on various platforms
- [**Configuration Guide**](CONFIGURATION.md) - Setting up AWS credentials and DNS settings
- [**Quick Start**](QUICK_START.md) - Get up and running in 5 minutes

### Platform-Specific Guides
- [**Ubiquiti Dream Machine Guide**](UDM_GUIDE.md) - Complete guide for UDM/UDR devices

### Reference
- [**Command Reference**](COMMANDS.md) - All available commands and options
- [**Troubleshooting**](TROUBLESHOOTING.md) - Common issues and solutions

## What is dddns?

dddns (Dynamic DNS) is a lightweight, efficient CLI tool that updates AWS Route53 DNS A records with your current public IP address. It's designed specifically for:

- 🏠 **Home networks** with dynamic IP addresses
- 🔒 **Ubiquiti Dream Machines** (UDM, UDR, UDM-Pro, etc.)
- ⚡ **Resource-constrained devices** (< 20MB memory usage)
- 🔄 **Automated updates** via cron

## Key Features

- ✅ **Simple** - Single binary, no dependencies
- ✅ **Secure** - Encrypted credential storage with device-specific keys
- ✅ **Efficient** - Minimal memory footprint, HTTP timeouts for reliability
- ✅ **Reliable** - IP change detection with persistent caching
- ✅ **Safe** - Proxy/VPN detection to prevent incorrect updates
- ✅ **Cron-friendly** - Quiet mode for unattended operation
- ✅ **Persistent** - Survives reboots and firmware updates on UDM

## Available Commands

```bash
# Configuration management
dddns config init         # Interactive configuration setup
dddns config check        # Validate configuration

# IP operations
dddns ip                  # Show current public IP address

# DNS updates
dddns update              # Update DNS record if IP changed
dddns update --dry-run    # Preview what would be updated
dddns update --force      # Force update even if IP unchanged
dddns update --quiet      # Suppress non-error output (for cron)

# Verification
dddns verify              # Check if DNS matches current IP

# Security
dddns secure enable       # Convert to encrypted config
dddns secure test         # Test device encryption
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