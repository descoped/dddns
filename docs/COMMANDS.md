# Command Reference

Complete reference for all dddns commands and options.

## Global Flags

These flags can be used with any command:

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--config <path>` | | Specify config file location | Platform-specific |
| `--help` | `-h` | Show help for command | |
| `--version` | `-v` | Show version information | |

## Commands Overview

```
dddns
├── config          # Configuration management
│   ├── init        # Create or update configuration
│   └── check       # Validate configuration
├── ip              # Show current public IP
├── update          # Update DNS record
├── verify          # Verify DNS matches current IP
└── secure          # Secure credential management
    ├── enable      # Convert to encrypted config
    └── test        # Test device encryption
```

## config

Manage dddns configuration files.

### config init

Create or update configuration file interactively.

```bash
dddns config init [flags]
```

**Flags:**
- `--force, -f` - Overwrite existing configuration
- `--interactive, -i` - Interactive setup (default: true)

**Examples:**
```bash
# Interactive setup (default)
dddns config init

# Non-interactive with defaults
dddns config init --interactive=false

# Force overwrite existing config
dddns config init --force
```

### config check

Validate configuration file and test AWS connectivity.

```bash
dddns config check
```

**Checks performed:**
- File exists and is readable
- File permissions are secure (600 or 400)
- Required fields are present
- AWS credentials are valid format
- Hosted zone ID format is correct
- TTL is within valid range (60-86400)

**Example:**
```bash
$ dddns config check
✓ Configuration valid
✓ File permissions: 600
✓ AWS credentials: configured
✓ Hosted Zone: Z1234567890ABC
✓ Hostname: home.example.com
✓ TTL: 300 seconds
```

## ip

Display current public IP address.

```bash
dddns ip
```

**Features:**
- Uses checkip.amazonaws.com for detection
- 10-second timeout for reliability
- No configuration required

**Example:**
```bash
$ dddns ip
203.0.113.42
```

## update

Update Route53 DNS A record with current IP address.

```bash
dddns update [flags]
```

**Flags:**
- `--dry-run` - Show what would be done without making changes
- `--force, -f` - Force update even if IP hasn't changed
- `--ip <address>` - Use specific IP instead of auto-detecting
- `--quiet, -q` - Suppress non-error output (for cron)

**Behavior:**
1. Detects current public IP (or uses --ip value)
2. Reads cached IP from file
3. Compares IPs - skips if unchanged (unless --force)
4. Checks for proxy/VPN (unless disabled in config)
5. Updates Route53 record
6. Updates cache file with new IP and timestamp

**Examples:**
```bash
# Normal update (only if IP changed)
dddns update

# Dry run to see what would happen
dddns update --dry-run
# Output: [DRY RUN] Would update home.example.com to 203.0.113.42

# Force update even if IP unchanged
dddns update --force

# Use specific IP
dddns update --ip 198.51.100.15

# Quiet mode for cron
dddns update --quiet

# Combine flags
dddns update --force --quiet
```

**Cache File Format:**
```yaml
last_known_ip: 203.0.113.42
last_updated: 2025-09-13T14:30:00Z
```

## verify

Check if DNS record matches current public IP.

```bash
dddns verify
```

**Output includes:**
- Current public IP
- Current DNS record value
- Match status
- Time since last update

**Example:**
```bash
$ dddns verify

=== DNS Verification ===

Your public IP:     203.0.113.42
Route53 record:     198.51.100.15

✗ DNS record doesn't match current IP
  Run 'dddns update' to fix this
```

**Exit Codes:**
- 0 - DNS matches current IP
- 1 - DNS doesn't match or error

## secure

Manage encrypted credential storage.

### secure enable

Convert plaintext config to encrypted format.

```bash
dddns secure enable
```

**Process:**
1. Reads existing plaintext config
2. Derives device-specific encryption key
3. Encrypts AWS credentials
4. Saves as config.secure with 0400 permissions
5. Securely wipes original plaintext file

**Example:**
```bash
$ dddns secure enable

=== Enable Secure Credential Storage ===

Current config: /home/user/.dddns/config.yaml
Secure config:  /home/user/.dddns/config.secure

✓ Credentials encrypted with device key
✓ Secure config created with permissions 0400
✓ Original config securely wiped

Next steps:
  1. Update your scripts to use: dddns --config /home/user/.dddns/config.secure
  2. Verify it works: dddns --config /home/user/.dddns/config.secure verify
```

### secure test

Test device encryption capabilities.

```bash
dddns secure test
```

**Tests performed:**
- Device key derivation
- Encryption/decryption cycle
- Memory wiping
- Platform detection

**Example:**
```bash
$ dddns secure test

=== Testing Device Encryption ===

✓ Device key derived (32 bytes)
✓ Test encryption successful
✓ Test decryption successful
✓ Memory wiped successfully

Device profile: udm
Hardware ID sources:
  - Machine ID: /etc/machine-id
  - Product UUID: /sys/class/dmi/id/product_uuid
  - CPU Info: /proc/cpuinfo
```

## Exit Codes

All commands use standard exit codes:

- `0` - Success
- `1` - Error (details in stderr)

## Configuration Precedence

1. Command-line flags (highest priority)
2. Configuration file
3. Default values (lowest priority)

## Cron Usage

For cron jobs, use the `--quiet` flag and redirect output:

```bash
# Standard cron entry
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1

# With secure config
*/30 * * * * /usr/local/bin/dddns --config /data/.dddns/config.secure update --quiet >> /var/log/dddns.log 2>&1
```

## Debugging

To debug issues, run commands without --quiet:

```bash
# Verbose output
dddns update

# Check what would be done
dddns update --dry-run

# Test configuration
dddns config check

# Test AWS connectivity
dddns verify
```

## Performance

Typical execution times:

| Command | Time | Notes |
|---------|------|-------|
| `ip` | ~200ms | Network request to checkip.amazonaws.com |
| `update` | ~500ms | Includes Route53 API call |
| `verify` | ~400ms | Route53 query only |
| `config check` | ~50ms | Local file validation |
| `secure enable` | ~100ms | Encryption overhead |

Memory usage: < 15MB for all operations

## Security Notes

1. **File Permissions**: Config files must have 600 or 400 permissions
2. **No Environment Variables**: AWS credentials must be in config file
3. **Device-Locked Encryption**: Secure configs only work on the device that created them
4. **HTTP Timeouts**: All network operations timeout after 10 seconds
5. **Secure Wiping**: Credentials are wiped from memory after use