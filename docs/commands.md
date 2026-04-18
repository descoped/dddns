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
├── config                # Configuration management
│   ├── init              # Create or update configuration
│   ├── check             # Validate configuration
│   ├── set-mode          # Switch UniFi run mode (cron|serve); rewrites boot script
│   └── rotate-secret     # Rotate the serve-mode shared secret
├── ip                    # Show current public IP
├── update                # Update DNS record
├── verify                # Verify DNS matches current IP
├── serve                 # Run the event-driven listener (UniFi serve mode)
│   ├── status            # Show the last request the listener handled
│   └── test              # Send a local Basic-Auth'd test request
└── secure                # Secure credential management
    ├── enable            # Convert to encrypted config
    └── test              # Test device encryption
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

### config set-mode

Switch between cron-driven updates and the serve-mode listener on UniFi Dream devices. Writes `/data/on_boot.d/20-dddns.sh` for the chosen mode.

```bash
dddns config set-mode {cron|serve} [flags]
```

**Arguments:**
- `cron` — install `/etc/cron.d/dddns` running `dddns update --quiet` every 30 minutes.
- `serve` — install `/etc/systemd/system/dddns.service` running the event-driven listener.

**Flags:**
- `--boot-path <path>` — override the boot-script destination (default `/data/on_boot.d/20-dddns.sh`).

**Behaviour:**
- Validates the active config. Switching to `serve` requires a populated `server:` block — run `dddns config rotate-secret --init` first.
- Writes the generated script with 0755 permissions. The script is idempotent and stops/starts the other mode's supervisor when run.
- Does **not** apply the change by itself. Run the script (or reboot) to take effect:

```bash
sudo /data/on_boot.d/20-dddns.sh
```

**Example:**
```bash
$ dddns config set-mode serve
Wrote /data/on_boot.d/20-dddns.sh (mode=serve)

To apply immediately, run as root:
  sudo /data/on_boot.d/20-dddns.sh
Or reboot the device — on_boot.d runs on every boot.
```

### config rotate-secret

Regenerate the 256-bit shared secret used by the serve-mode listener to authenticate UniFi's `inadyn` push. The new secret is printed exactly once — copy it into the UniFi UI's Dynamic DNS Password field.

```bash
dddns config rotate-secret [flags]
```

**Flags:**
- `--init` — create a default `server:` block (loopback bind, `127.0.0.0/8` allowlist) if none exists. The UniFi installer uses this internally when first enabling serve mode.
- `--quiet` — print only the new secret on stdout (for scripting).

**Behaviour:**
1. Loads the active config (plaintext or secure).
2. Generates 32 bytes via `crypto/rand`, hex-encoded to 64 lowercase characters.
3. Writes the secret back, preserving the on-disk format (plaintext `config.yaml` or encrypted `config.secure`).
4. Appends a `rotate-secret` entry to the audit log.

**Example:**
```bash
$ dddns config rotate-secret
=================================================================
  New shared secret generated (2026-04-18T12:34:56Z)
=================================================================

  0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

=================================================================
  Written to: /data/.dddns/config.yaml

  Paste this value into the UniFi Network Controller's
  Dynamic DNS Password field before the next IP change,
  otherwise the next request will fail auth.
=================================================================
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

## serve

Start the event-driven HTTP listener that accepts dyndns-v2 updates from UniFi's on-device `inadyn`. Binds to `cfg.Server.Bind` (default `127.0.0.1:53353`) and pushes the router's authoritative WAN IP to Route53 on each valid request.

```bash
dddns serve
```

**Behaviour:**
- Blocks — exits on SIGINT/SIGTERM. On UniFi devices the command runs under `systemd`, supervised by `dddns.service`.
- Fail-closed startup: refuses to start if `server.bind`, `server.shared_secret` (or `server.secret_vault`), `server.allowed_cidrs`, or `cfg.hostname` are missing.
- Never trusts the `myip` query parameter — reads the WAN interface directly via `internal/wanip` and uses that for the Route53 UPSERT.

Serve mode is the alternative to cron polling and is mutually exclusive with it. Choose with `dddns config set-mode {cron|serve}`. See the [UDM Guide](udm-guide.md) for end-to-end setup.

### serve status

Print the last request the listener handled: timestamp, remote address, auth outcome, action, and error (if any). Reads `<data-dir>/serve-status.json` written atomically by the server on every request.

```bash
dddns serve status
```

**Example:**
```bash
$ dddns serve status
Status file:    /data/.dddns/serve-status.json
Last request:   2026-04-18T12:30:00Z
Remote:         127.0.0.1:44022
Auth outcome:   ok
Action:         nochg-cache
```

Exits non-zero when the status file is missing — typically because `dddns serve` has not handled any requests yet.

### serve test

Craft a Basic-Auth'd dyndns-v2 `GET` to the listener using the shared secret from config. Prints the HTTP status code and response body. Use after rotating the secret, switching modes, or whenever the UniFi UI's DDNS status turns red.

```bash
dddns serve test [flags]
```

**Flags:**
- `--hostname <name>` — override `cfg.Hostname` in the request (default uses config value).
- `--ip <address>` — the `myip` query parameter (default `1.2.3.4`). The handler ignores this for the actual UPSERT — it's only here for wire-level testing.

**Exit codes:**
- `0` — response body starts with `good` or `nochg`.
- non-zero — any other dyndns code, HTTP 4xx/5xx, or network error.

**Example:**
```bash
$ dddns serve test
HTTP 200
Body: good 203.0.113.42
```

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