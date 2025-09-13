# Ubiquiti Dream Machine Guide

Complete guide for running dddns on Ubiquiti Dream Machine devices.

## Table of Contents
- [Supported Devices](#supported-devices)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
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
   - AWS credentials ready

## Installation

### Quick Install

```bash
# One-line installation
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash
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
   curl -O https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh
   chmod +x install.sh
   
   # Check environment first
   ./install.sh --check-only
   
   # Install
   ./install.sh
   ```

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

- **HTTPS (443)**: to AWS Route53 API
- **HTTP (80)**: to ip-api.com for proxy detection
- **DNS (53)**: for hostname resolution

No inbound ports required.

### VPN Considerations

If using split-tunnel VPN on your UDM:

```yaml
# /data/.dddns/config.yaml
skip_proxy_check: true  # Prevents false proxy detection
```

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

### View Logs

```bash
# Real-time logs
tail -f /var/log/dddns.log

# Last 50 entries
tail -50 /var/log/dddns.log

# Today's updates
grep "$(date +%Y-%m-%d)" /var/log/dddns.log

# Check for errors
grep -i error /var/log/dddns.log
```

### Check Cron Execution

```bash
# View cron jobs
cat /etc/cron.d/dddns

# Check cron logs
grep CRON /var/log/messages | grep dddns

# Verify cron is running
ps aux | grep cron
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

### Custom Update Interval

Default is 30 minutes. To change:

```bash
# Edit cron schedule
vi /data/on_boot.d/20-dddns.sh

# Change the cron line:
# Every 15 minutes
*/15 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1

# Every hour
0 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1

# Run the script to apply
/data/on_boot.d/20-dddns.sh
```

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
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash -s -- --uninstall
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash
```

### Getting Help

1. **Check logs first**
   ```bash
   tail -100 /var/log/dddns.log
   ```

2. **Run environment check**
   ```bash
   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash -s -- --check-only
   ```

3. **Test components**
   ```bash
   # Test IP resolution
   curl -s https://checkip.amazonaws.com
   
   # Test AWS access
   aws route53 list-hosted-zones --profile your-profile
   
   # Test dddns
   dddns update --dry-run
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

- [Configuration Guide](CONFIGURATION.md)
- [Command Reference](COMMANDS.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)