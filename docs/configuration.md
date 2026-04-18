# Configuration Guide

This guide covers all configuration options for dddns.

## Table of Contents
- [Configuration File](#configuration-file)
- [File Permissions](#file-permissions)
- [AWS Credentials](#aws-credentials)
- [DNS Settings](#dns-settings)
- [Operational Settings](#operational-settings)
- [IP Source Selection](#ip-source-selection)
- [Serve-Mode (`server:`) Block](#serve-mode-server-block)
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
ip_source: auto                   # auto | local | remote (see IP Source Selection)

# Serve-mode block (only present if `dddns config set-mode serve` was run)
server:
  bind: "127.0.0.1:53353"
  shared_secret: "<64 hex chars>"
  allowed_cidrs: ["127.0.0.0/8"]
  wan_interface: ""               # empty = auto-detect
```

## File Permissions

dddns enforces `0600` on the plaintext config at load time. As of v0.2.0 any command that loads the config — including `dddns config check`, `dddns update`, and `dddns serve` — refuses to read `config.yaml` whose mode is anything other than `-rw-------`.

```bash
# Correct
chmod 600 ~/.dddns/config.yaml         # Linux / macOS
chmod 600 /data/.dddns/config.yaml     # UDM / UDR

# Secure (encrypted) config is stricter still
chmod 400 ~/.dddns/config.secure
```

If permissions are wrong, the error identifies the offending file and the fix:

```text
config file /data/.dddns/config.yaml has permissions 644, must be 600 (chmod 600 /data/.dddns/config.yaml)
```

The file holds AWS credentials, so a looser mode is a local privilege-escalation vector on any host where dddns shares a user account with untrusted services. The check is deliberately non-negotiable.

## AWS Credentials

**IMPORTANT**: For security reasons, dddns does NOT use environment variables or IAM roles. Credentials must be explicitly configured in the config file.

### Required AWS Permissions

The minimum workable policy grants zone-wide `ListResourceRecordSets` and `ChangeResourceRecordSets`:

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

For production use — especially with serve mode — prefer the **scoped** policy documented in the [AWS Setup Guide](aws-setup.md). It restricts the IAM user to `UPSERT` on a single record name and type via Route53 condition keys, so stolen credentials cannot delete the record, change the TTL, or touch any other record in the zone.

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

## IP Source Selection

`ip_source` controls where dddns obtains the current public IPv4 for a cron-mode update. Three values are accepted:

| Value    | Behaviour                                                                                 |
|----------|-------------------------------------------------------------------------------------------|
| `auto`   | Default. Resolves to `local` on UniFi profile detection, `remote` everywhere else.        |
| `local`  | Reads the WAN interface directly via the OS. No third-party round trip.                   |
| `remote` | Calls `checkip.amazonaws.com`. The pre-v0.2.0 default on every platform.                  |

```yaml
ip_source: auto   # recommended; mode-aware
```

Serve mode always uses the local interface regardless of this setting — the `myip` query parameter from `inadyn` is never trusted, and the authoritative local IP is also faster and available during WAN flaps when outbound connectivity may not be.

The `local` path rejects RFC1918 space, CGNAT (`100.64.0.0/10`), link-local, and IPv6 — if the first address on the detected interface is any of those, dddns falls back to scanning up interfaces for a publicly-routable IPv4. This covers devices like UDR7 where policy-based routing moves the default route out of the main table.

## Serve-Mode (`server:`) Block

Populated by `dddns config rotate-secret --init` (the UniFi installer does this automatically when serve mode is selected). Absent from the config file for cron-mode installs; `dddns serve` refuses to start if it's empty.

```yaml
server:
  bind: "127.0.0.1:53353"        # loopback-only by default
  shared_secret: "..."            # 64 hex chars; rotated with `dddns config rotate-secret`
  allowed_cidrs:                  # RemoteAddr allowlist; fail-closed when empty
    - "127.0.0.0/8"
  wan_interface: ""               # empty = auto-detect; set to e.g. "eth4" to pin
  audit_log: "/var/log/dddns-audit.log"   # optional; default is platform-specific
```

**Fields:**

- `bind` — host:port the listener binds to. Default `127.0.0.1:53353` (loopback). LAN reachability is explicit opt-in (`0.0.0.0:53353`) and requires widening `allowed_cidrs` — do not use the whole RFC1918 space.
- `shared_secret` — the Basic Auth password `inadyn` sends. Generated by the installer, rotated via `dddns config rotate-secret`. In encrypted configs the field is named `secret_vault` and holds the AES-256-GCM ciphertext.
- `allowed_cidrs` — `RemoteAddr` CIDR allowlist, enforced before auth. Empty list → server refuses to start. The default `127.0.0.0/8` pairs with the loopback bind.
- `wan_interface` — pin the WAN interface name (e.g. `eth4`, `pppoe-wan0`). Empty string auto-detects from `/proc/net/route` and falls back to interface scanning.
- `audit_log` — JSONL audit log path; rotated at 10 MB.

Serve mode is only meaningful on UniFi Dream devices. See the [UDM Guide](udm-guide.md) for installation and the UniFi UI values.

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