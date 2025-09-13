# Configuration Guide

This guide covers all configuration options for dddns.

## Table of Contents
- [Configuration File](#configuration-file)
- [AWS Credentials](#aws-credentials)
- [DNS Settings](#dns-settings)
- [Operational Settings](#operational-settings)
- [Secure Credentials](#secure-credentials)
- [Command-Line Flags](#command-line-flags)
- [Configuration Priority](#configuration-priority)
- [Examples](#examples)

## Configuration File

dddns uses a YAML configuration file to store settings. The default locations are automatically determined based on your system:

- **UDM/UDR**: `/data/.dddns/config.yaml` (persistent across reboots)
- **Linux/macOS**: `~/.dddns/config.yaml`
- **Custom**: Use `--config /path/to/config.yaml`

### Creating Configuration

```bash
# Interactive configuration setup
dddns config init

# Non-interactive with defaults
dddns config init --interactive=false

# Check configuration validity
dddns config check
```

### Full Configuration Example

```yaml
# dddns Configuration File

# AWS Settings (REQUIRED - no environment variables for security)
aws_region: "us-east-1"           # AWS region for Route53
aws_access_key: "AKIA..."         # Your AWS Access Key ID
aws_secret_key: "..."              # Your AWS Secret Access Key

# DNS Settings (required)
hosted_zone_id: "Z1234567890ABC"  # Your Route53 Hosted Zone ID
hostname: "home.example.com"      # Domain name to update
ttl: 300                          # Time-to-live in seconds (60-86400)

# Operational Settings
ip_cache_file: "/data/.dddns/last-ip.txt"  # Auto-set based on platform
skip_proxy_check: false                     # Skip proxy/VPN detection
```

## AWS Credentials

**IMPORTANT**: For security reasons, dddns does NOT use environment variables or IAM roles. Credentials must be explicitly configured in the config file.

### Required AWS Permissions

Create an IAM user with minimal permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "route53:ListResourceRecordSets",
        "route53:ChangeResourceRecordSets"
      ],
      "Resource": "arn:aws:route53:::hostedzone/YOUR_ZONE_ID"
    }
  ]
}
```

### Finding Your Hosted Zone ID

```bash
# Using AWS CLI
aws route53 list-hosted-zones --query "HostedZones[?Name=='example.com.']"

# Or check AWS Console
# Route53 → Hosted zones → Select your domain → Copy Zone ID
```

## DNS Settings

### hostname
The fully qualified domain name to update. Must be an A record in your hosted zone.

Example: `home.example.com`, `vpn.mydomain.org`

### ttl
Time-to-live in seconds. Lower values mean faster propagation but more DNS queries.

- **Minimum**: 60 seconds
- **Recommended**: 300 seconds (5 minutes)
- **Maximum**: 86400 seconds (24 hours)

## Operational Settings

### ip_cache_file
Location where the last known IP is stored. This file persists between runs to detect IP changes.

**Default locations** (automatically set based on detected platform):
- **UDM/UDR**: `/data/.dddns/last-ip.txt` (survives reboots)
- **Linux/macOS**: `~/.dddns/last-ip.txt`

The cache file contains:
```yaml
last_known_ip: 203.0.113.42
last_updated: 2025-09-13T14:30:00Z
```

### skip_proxy_check
When `false` (default), dddns checks if your IP is from a proxy/VPN and skips updates to prevent setting incorrect IPs.

Set to `true` if:
- You're behind a corporate proxy
- You use a VPN but want to update anyway
- The proxy detection service is unavailable

## Secure Credentials

For enhanced security, dddns supports encrypted credential storage using device-specific encryption.

### Enable Secure Storage

```bash
# Convert existing config to secure format
dddns secure enable

# This will:
# 1. Encrypt AWS credentials using device-specific key
# 2. Create config.secure file with encrypted data
# 3. Securely wipe the original plaintext config
# 4. Set file permissions to 0400 (read-only)
```

### Using Secure Config

```bash
# Use secure config file
dddns --config ~/.dddns/config.secure update

# Or set as default in cron
*/30 * * * * /usr/local/bin/dddns --config /data/.dddns/config.secure update --quiet
```

### Security Features
- **Device-locked**: Encrypted with hardware-specific identifiers
- **Secure permissions**: 0400 (read-only by owner)
- **Memory protection**: Credentials wiped from memory after use
- **No plaintext storage**: Original config securely deleted

## Command-Line Flags

Flags override configuration file settings:

```bash
# Global flags
--config <path>       # Use specific config file
--quiet, -q          # Suppress non-error output (for cron)

# Update command flags
--force, -f          # Force update even if IP unchanged
--dry-run            # Show what would be done without changes
--ip <address>       # Use specific IP instead of auto-detecting

# Config command flags
--interactive, -i    # Interactive setup (default: true)
--force, -f         # Overwrite existing config
```

## Configuration Priority

Settings are applied in this order (highest to lowest priority):

1. Command-line flags
2. Configuration file
3. Default values

## Examples

### Basic Home Network Setup

```yaml
aws_region: "us-east-1"
aws_access_key: "AKIAIOSFODNN7EXAMPLE"
aws_secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
hosted_zone_id: "Z1234567890ABC"
hostname: "home.mydomain.com"
ttl: 300
```

### UDM Router Configuration

```yaml
aws_region: "eu-west-1"
aws_access_key: "AKIAIOSFODNN7EXAMPLE"
aws_secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
hosted_zone_id: "Z0987654321XYZ"
hostname: "router.example.org"
ttl: 600
ip_cache_file: "/data/.dddns/last-ip.txt"  # Persistent location
skip_proxy_check: false
```

### Cron Setup

```bash
# Standard setup - updates every 30 minutes
*/30 * * * * /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1

# Quiet mode for cron (only logs errors and actual updates)
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1

# With secure config
*/30 * * * * /usr/local/bin/dddns --config /data/.dddns/config.secure update --quiet >> /var/log/dddns.log 2>&1
```

## File Permissions

For security, configuration files must have restricted permissions:

```bash
# Standard config
chmod 600 ~/.dddns/config.yaml  # Read/write by owner only

# Secure config (automatically set)
chmod 400 ~/.dddns/config.secure  # Read-only by owner
```

dddns will warn and refuse to run if permissions are too open.

## Troubleshooting

### Config not found
```bash
# Check where dddns is looking
dddns config check

# Specify config explicitly
dddns --config /path/to/config.yaml update
```

### Permission denied
```bash
# Fix permissions
chmod 600 ~/.dddns/config.yaml
```

### Invalid credentials
```bash
# Test AWS access
aws sts get-caller-identity --profile dddns

# Verify IAM permissions
aws route53 list-resource-record-sets --hosted-zone-id YOUR_ZONE_ID
```

## Migration from Environment Variables

If migrating from the bash script version:

```bash
# Old (bash script with env vars)
export AWS_PROFILE=route53-updater
export HOSTED_ZONE_ID=Z1234567890ABC

# New (dddns with config file)
dddns config init  # Interactive setup
# Enter your AWS credentials when prompted
```

## Best Practices

1. **Use secure storage** for production deployments
2. **Set restrictive permissions** on config files (600 or 400)
3. **Use --quiet flag** in cron to reduce log noise
4. **Regular verification** with `dddns verify` command
5. **Monitor logs** for update failures
6. **Backup config** before updates (especially .secure files)