# Ubiquiti Dream Machine Guide

Complete guide for running dddns on Ubiquiti Dream Machine devices.

## Table of Contents
- [Supported Devices](#supported-devices)
- [Prerequisites](#prerequisites)
- [Run Modes](#run-modes)
- [Installation](#installation)
- [Serve Mode](#serve-mode)
- [Switching Modes](#switching-modes)
- [UniFi OS Compatibility](#unifi-os-compatibility)
- [Persistence Across Updates](#persistence-across-updates)
- [Network Configuration](#network-configuration)
- [Monitoring](#monitoring)
- [Advanced Configuration](#advanced-configuration)
- [Troubleshooting](#troubleshooting)

## Supported Devices

| Model | CPU | Architecture | UniFi OS | Status |
|-------|-----|--------------|----------|---------|
| **UDM** | ARM Cortex-A57 | ARM64 | 2.x-3.x | ✅ Fully Supported |
| **UDM-Pro** | ARM Cortex-A57 | ARM64 | 2.x-3.x | ✅ Fully Supported |
| **UDM-SE** | ARM Cortex-A57 | ARM64 | 2.x-3.x | ✅ Fully Supported |
| **UDM Pro Max** | Enhanced ARM | ARM64 | 3.x | ✅ Fully Supported |
| **UDR** | Dual-core ARM | ARM64 | 2.x-3.x | ✅ Fully Supported |
| **UDR7** | Cortex-A53 | ARM64 | 3.x | ✅ Fully Supported |

## Prerequisites

Before installing dddns on your UDM:

1. **SSH Access Enabled**
   - UniFi Network Controller → Settings → System → Advanced → Enable SSH
   - Set a strong SSH password

2. **Root Access**
   ```bash
   ssh root@<your-udm-ip>
   ```

3. **Internet Connectivity**
   - Ensure your UDM can reach github.com and AWS Route53

4. **AWS Account Setup**
   - Route53 hosted zone configured
   - AWS credentials ready (see the [AWS Setup Guide](aws-setup.md) for the scoped IAM policy)

## Run Modes

dddns supports two mutually-exclusive run modes on UniFi Dream devices. Pick one at install time; switch later with `dddns config set-mode`.

| Mode  | Trigger              | Boot artefact                   | Log file                     | Typical latency |
|-------|----------------------|---------------------------------|------------------------------|-----------------|
| cron  | `/etc/cron.d/dddns` every 30 min | `/data/on_boot.d/20-dddns.sh` (installs cron) | `/var/log/dddns.log` | up to 30 min |
| serve | UniFi UI "Custom" Dynamic DNS → `inadyn` → local HTTP | `/data/on_boot.d/20-dddns.sh` (starts supervised `dddns serve` loop) | `/var/log/dddns-server.log` + `/var/log/dddns-audit.log` | seconds |

**cron mode** is the safe default — polls your public IP, compares against the cached value, UPSERTs Route53 if changed. No inbound sockets, no secrets on the wire.

**serve mode** replaces the cron entry with a long-running `dddns serve` listener bound to `127.0.0.1:53353`. UniFi's built-in `inadyn` (configured via the Network Controller's Dynamic DNS dialog) calls the listener on every WAN IP change, and the handler reads the authoritative IP directly from the WAN interface before calling Route53. Faster, but introduces a new on-device HTTP surface that must be kept loopback-only. See the [Serve Mode](#serve-mode) section below for full setup.

The installer asks which one you want; pass `--mode cron` or `--mode serve` to skip the prompt.

## Installation

### Quick Install

```bash
# One-line installation
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
```

### Step-by-Step Installation

1. **Connect to your UDM**
   ```bash
   ssh root@192.168.1.1  # Replace with your UDM IP
   ```

2. **Check environment**
   ```bash
   # Check your device
   uname -a
   
   # Check available space
   df -h /data
   
   # Check existing boot scripts
   ls -la /data/on_boot.d/
   ```

3. **Run installer**
   ```bash
   # Download installer
   curl -O https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh
   chmod +x install-on-unifi-os.sh

   # Interactive (prompts for mode)
   ./install-on-unifi-os.sh

   # Non-interactive
   ./install-on-unifi-os.sh --mode cron    # polling every 30 min
   ./install-on-unifi-os.sh --mode serve   # event-driven via UniFi UI
   ```

   The installer verifies the binary's SHA-256 against the release's
   `checksums.txt` before extracting — a tampered or corrupt download
   aborts the install rather than running.

4. **Configure AWS credentials**
   ```bash
   # Option 1: Use AWS CLI profile
   aws configure --profile route66dns
   
   # Option 2: Edit config directly
   vi /data/.dddns/config.yaml
   ```

5. **Test installation**
   ```bash
   # Check version
   dddns --version
   
   # Test IP resolution
   dddns ip
   
   # Test update (dry run)
   dddns update --dry-run
   ```

## Serve Mode

Serve mode turns the router's built-in dynamic DNS client (`inadyn`) into dddns's trigger. When the WAN IP changes, UniFi OS fires an HTTP request to the local `dddns serve` listener, which reads the authoritative IP from the WAN interface and pushes to Route53 — no 30-minute delay, no third-party IP-lookup round trip.

### Installing

```bash
./install-on-unifi-os.sh --mode serve
```

The installer:
1. Generates a 256-bit shared secret and writes it to `config.yaml` (or `config.secure` if secure mode is already enabled).
2. Creates the `server:` block with loopback-only bind (`127.0.0.1:53353`) and a `127.0.0.0/8` CIDR allowlist.
3. Writes the on_boot.d script that starts a supervised `dddns serve` loop.
4. Prints a framed block with the UniFi UI values to paste.

Copy the printed secret immediately — it's not shown again. To rotate later, run `dddns config rotate-secret` (see [Rotating the Shared Secret](#rotating-the-shared-secret) below).

### Configuring the UniFi Dynamic DNS UI

Settings → Internet → Dynamic DNS → **Create Dynamic DNS**:

| Field     | Value                                                   |
|-----------|---------------------------------------------------------|
| Service   | `Custom`                                                |
| Hostname  | must match `cfg.Hostname` (e.g. `home.example.com`)     |
| Username  | `dddns` (any value — the handler ignores the username)  |
| Password  | the shared secret printed by the installer             |
| Server    | `127.0.0.1:53353/nic/update?hostname=%h&myip=%i`        |

Click **Apply**. UniFi OS fires `inadyn` on every subsequent WAN IP change.

### Testing the Listener

From an SSH session on the router:

```bash
dddns serve test
```

This crafts a Basic-Auth'd request to `127.0.0.1:53353`, hitting your own handler. Expected output on a healthy install:

```
HTTP 200
Body: good <your-wan-ip>
```

Exit code 0 on `good` or `nochg`; non-zero otherwise. Useful after a rotation or if UniFi UI status turns red.

### Status Summary

```bash
dddns serve status
```

Prints the last request the listener handled: timestamp, remote address, auth outcome, action, and error (if any). The file it reads is `/data/.dddns/serve-status.json`, refreshed atomically on every request.

### Rotating the Shared Secret

```bash
dddns config rotate-secret
```

Generates a fresh 256-bit secret, writes it back to config (re-encrypting if `.secure`), and prints the new value. Then paste the new secret into the UniFi UI's Password field — the next IP change will fail auth until you do.

`dddns config rotate-secret --init` creates the `server:` block if one doesn't exist yet (what the installer runs internally). `--quiet` prints only the secret on stdout, for scripting.

### Log Files

| File                           | What's in it                                           |
|--------------------------------|--------------------------------------------------------|
| `/var/log/dddns-server.log`    | stdout/stderr from the supervised `dddns serve` loop — startup, shutdown, unexpected errors |
| `/var/log/dddns-audit.log`     | JSONL, one line per request (ts, remote, hostname, myip_claimed, myip_verified, auth, action, route53_change_id, error) |
| `/data/.dddns/serve-status.json` | Last-request summary (overwritten; `dddns serve status` reads this) |

Follow both logs during a test:

```bash
tail -f /var/log/dddns-server.log /var/log/dddns-audit.log
```

The audit log rotates itself at 10 MB to `.old` (one keep). A `myip_claimed` value that differs from `myip_verified` is a strong anomaly signal — the handler always uses the verified (local interface) IP for the Route53 upsert, so the difference is captured for review but never acted on.

## Switching Modes

The modes are mutually exclusive. Switch at any time with:

```bash
dddns config set-mode cron
dddns config set-mode serve
```

The command rewrites `/data/on_boot.d/20-dddns.sh` for the target mode. It does not apply the change immediately — run the script or reboot:

```bash
sudo /data/on_boot.d/20-dddns.sh
```

The generated script is idempotent: switching to `cron` removes any stale serve loop (`pkill -f "dddns serve"`) and installs the cron entry; switching to `serve` removes `/etc/cron.d/dddns` and starts the supervised loop. Re-running it repeatedly converges on the target state.

Switching to `serve` requires `cfg.Server` to be populated. If the block isn't there, run `dddns config rotate-secret --init` first.

## UniFi OS Compatibility

### UniFi OS 2.x

- Full compatibility
- Podman support (not used by dddns)
- Standard `/data` persistence

### UniFi OS 3.x

- Full compatibility
- No podman support (doesn't affect dddns)
- Enhanced security features
- May require reinstalling on-boot-script after major updates

### Checking Your Version

```bash
# Check UniFi OS version
cat /etc/unifi-os/unifi-os.conf

# Check firmware version
ubnt-device-info firmware

# Check kernel version
uname -r
```

## Persistence Across Updates

dddns is designed to survive:
- ✅ Device reboots
- ✅ Minor firmware updates
- ✅ Configuration changes
- ⚠️ Major firmware updates (may need reinstall of on-boot-script)

### How Persistence Works

1. **Binary Location**: `/data/dddns/` - Survives all updates
2. **Configuration**: `/data/.dddns/` - Survives all updates
3. **Boot Script**: `/data/on_boot.d/` - Executes on every boot
4. **Cron Job**: Recreated on boot via script

### After Firmware Update

If dddns stops working after a major firmware update:

```bash
# Reinstall on-boot-script
curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash

# Run dddns boot script
/data/on_boot.d/20-dddns.sh

# Verify
dddns --version
```

## Network Configuration

### Firewall Rules

dddns only makes outbound connections:

- **HTTPS (443)**: to AWS Route53 API (`route53.amazonaws.com`) and the public-IP lookup (`checkip.amazonaws.com`)
- **DNS (53)**: for hostname resolution

No inbound ports required.

### Multi-WAN Setup

For UDM with multiple WAN connections:

```bash
# Check which interface is primary
ip route show default

# Force specific interface (if needed)
# Edit /data/on_boot.d/20-dddns.sh to add:
export BIND_INTERFACE=eth8  # Your WAN interface
```

## Monitoring

### Log Files at a Glance

| File                             | Mode  | Purpose                                             |
|----------------------------------|-------|-----------------------------------------------------|
| `/var/log/dddns.log`             | cron  | `dddns update` stdout/stderr on each cron tick     |
| `/var/log/dddns-boot.log`        | both  | One line per boot-script execution                 |
| `/var/log/dddns-server.log`      | serve | `dddns serve` lifecycle (startup, crashes, restarts) |
| `/var/log/dddns-audit.log`       | serve | JSONL trail of every request the listener handled  |

The two serve-mode logs answer different questions. The *server* log tells you whether the daemon is alive; the *audit* log tells you what the daemon did for each caller.

### View Logs

Cron mode:

```bash
tail -f /var/log/dddns.log
grep -i error /var/log/dddns.log
grep "$(date +%Y-%m-%d)" /var/log/dddns.log
```

Serve mode:

```bash
# Operational log — daemon lifecycle
tail -f /var/log/dddns-server.log

# Audit log — per-request structured trail (JSONL)
tail -f /var/log/dddns-audit.log
tail -n 50 /var/log/dddns-audit.log | jq -c '{ts,remote,auth,action,error}'
```

### Check Cron Execution (cron mode)

```bash
# View cron entry
cat /etc/cron.d/dddns

# Check cron logs
grep CRON /var/log/messages | grep dddns

# Verify cron is running
ps aux | grep cron
```

### Check the Listener (serve mode)

```bash
# Is the daemon running?
pgrep -laf "dddns serve"

# What's the last request it handled?
dddns serve status

# Can we reach it from the router itself?
dddns serve test
```

### Monitor IP Changes

```bash
# Current cached IP
cat /data/.dddns/last-ip.txt

# Current public IP
dddns ip

# Current DNS record
dig +short $(grep hostname /data/.dddns/config.yaml | cut -d'"' -f2)
```

### Create Monitoring Script

```bash
cat > /data/on_boot.d/21-dddns-monitor.sh << 'EOF'
#!/bin/bash
# Monitor dddns and alert on failures

LOG_FILE="/var/log/dddns.log"
ALERT_FILE="/tmp/dddns-alert"

# Check for recent errors
if grep -q "ERROR\|Failed" "$LOG_FILE" 2>/dev/null; then
    if [ ! -f "$ALERT_FILE" ]; then
        echo "dddns errors detected at $(date)" > "$ALERT_FILE"
        # Add notification method here (email, webhook, etc.)
    fi
else
    rm -f "$ALERT_FILE"
fi
EOF
chmod +x /data/on_boot.d/21-dddns-monitor.sh
```

## Advanced Configuration

### Custom Update Interval (cron mode)

The default 30-minute interval lives in `/data/on_boot.d/20-dddns.sh`, which is generated by `dddns config set-mode cron`. Editing that file by hand works until the next `set-mode` call overwrites it. For a lasting change, add a custom cron entry alongside the managed one:

```bash
cat > /etc/cron.d/dddns-fast << 'CRON'
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
*/15 * * * * root /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
CRON
rm -f /etc/cron.d/dddns   # optional — disable the 30-min default
/etc/init.d/cron restart
```

Serve mode users should ignore this section — there is no polling interval to tune; updates happen on every WAN IP change as observed by `inadyn`.

### Multiple Domains

To update multiple domains:

```bash
# Create separate configs
cat > /data/.dddns/domain1.yaml << EOF
hosted_zone_id: "Z1111111111111"
hostname: "home.example.com"
# ... other settings
EOF

cat > /data/.dddns/domain2.yaml << EOF
hosted_zone_id: "Z2222222222222"
hostname: "vpn.example.net"
# ... other settings
EOF

# Update boot script
cat > /data/on_boot.d/20-dddns-multi.sh << 'EOF'
#!/bin/bash
# Update multiple domains
*/30 * * * * root /usr/local/bin/dddns update --config /data/.dddns/domain1.yaml >> /var/log/dddns.log 2>&1
*/30 * * * * root /usr/local/bin/dddns update --config /data/.dddns/domain2.yaml >> /var/log/dddns.log 2>&1
EOF
```

### Log Rotation

Prevent logs from filling up storage:

```bash
cat > /data/on_boot.d/22-dddns-logrotate.sh << 'EOF'
#!/bin/bash
# Rotate dddns logs

LOG_FILE="/var/log/dddns.log"
MAX_SIZE=10485760  # 10MB

if [ -f "$LOG_FILE" ]; then
    SIZE=$(stat -c%s "$LOG_FILE")
    if [ $SIZE -gt $MAX_SIZE ]; then
        mv "$LOG_FILE" "$LOG_FILE.old"
        touch "$LOG_FILE"
        echo "[$(date)] Log rotated" > "$LOG_FILE"
    fi
fi
EOF
chmod +x /data/on_boot.d/22-dddns-logrotate.sh
```

### Integration with UniFi Alerts

```bash
# Create alert on IP change
cat > /data/on_boot.d/23-dddns-alert.sh << 'EOF'
#!/bin/bash
# Alert on IP changes

CACHE_FILE="/data/.dddns/last-ip.txt"
ALERT_CACHE="/tmp/last-alert-ip"

if [ -f "$CACHE_FILE" ]; then
    CURRENT_IP=$(cat "$CACHE_FILE")
    if [ -f "$ALERT_CACHE" ]; then
        LAST_ALERT=$(cat "$ALERT_CACHE")
        if [ "$CURRENT_IP" != "$LAST_ALERT" ]; then
            logger -t dddns "Public IP changed to $CURRENT_IP"
            echo "$CURRENT_IP" > "$ALERT_CACHE"
        fi
    else
        echo "$CURRENT_IP" > "$ALERT_CACHE"
    fi
fi
EOF
chmod +x /data/on_boot.d/23-dddns-alert.sh
```

## Troubleshooting

### Common Issues

#### dddns command not found

```bash
# Recreate symlink
ln -sf /data/dddns/dddns /usr/local/bin/dddns

# Run boot script
/data/on_boot.d/20-dddns.sh
```

#### Configuration not found

```bash
# Check config location
ls -la /data/.dddns/

# Recreate if missing
dddns config init
```

#### AWS credentials not working

```bash
# Check AWS profile
export AWS_CONFIG_FILE=/root/.aws/config
export AWS_SHARED_CREDENTIALS_FILE=/root/.aws/credentials
aws route53 list-hosted-zones --profile route66dns
```

#### No updates happening

```bash
# Check cron
service cron status
cat /etc/cron.d/dddns

# Restart cron
/etc/init.d/cron restart

# Run manually
dddns update
```

### Debug Mode

```bash
# Enable debug output
DEBUG=1 dddns update

# Check boot script logs
tail -f /var/log/dddns-boot.log

# System logs
dmesg | tail
cat /var/log/messages | grep dddns
```

### Reset Installation

```bash
# Complete removal and reinstall
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash -s -- --uninstall
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
```

### Getting Help

1. **Check logs first**
   ```bash
   tail -100 /var/log/dddns.log
   ```

2. **Test components**
   ```bash
   # Cron mode: dry-run the full update flow
   dddns update --dry-run

   # Serve mode: exercise the listener locally
   dddns serve test

   # Independent: test IP resolution
   curl -s https://checkip.amazonaws.com

   # Independent: verify AWS access
   aws route53 list-hosted-zones --profile your-profile
   ```

## Best Practices

1. **Regular Monitoring**
   - Check logs weekly
   - Verify DNS updates are working
   - Monitor disk space in `/data`

2. **Security**
   - Use AWS IAM with minimal permissions
   - Protect config files: `chmod 600 /data/.dddns/config.yaml`
   - Regularly update dddns

3. **Backup Configuration**
   ```bash
   # Backup config
   cp -r /data/.dddns /data/.dddns.backup
   
   # Backup boot scripts
   tar -czf /data/boot-scripts-backup.tar.gz /data/on_boot.d/
   ```

4. **Update Strategy**
   - Test updates on non-production first
   - Keep installer script for easy reinstall
   - Document any customizations

## Performance Impact

dddns has minimal impact on UDM:

- **CPU**: < 1% (runs for ~1 second every 30 minutes)
- **Memory**: < 20MB when running
- **Disk**: < 10MB total footprint
- **Network**: ~5KB per update

## Next Steps

- [Configuration Guide](configuration.md)
- [Command Reference](commands.md)
- [Troubleshooting Guide](troubleshooting.md)