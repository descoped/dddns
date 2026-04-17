# Raspberry Pi Support for dddns

## Overview

dddns fully supports Raspberry Pi 4 and 5, providing lightweight DNS updates for home labs and IoT projects. The binary is optimized for ARM processors and uses minimal resources, perfect for Pi deployments.

## Compatibility Matrix

| Model | Architecture | dddns Binary | Status | Memory Usage |
|-------|-------------|--------------|--------|--------------|
| **Raspberry Pi 5** | ARM64 (64-bit) | `dddns-linux-arm64` | ✅ Full Support | ~12MB |
| **Raspberry Pi 4** | ARM64/ARMv7 | `dddns-linux-arm64` or `arm` | ✅ Full Support | ~12MB |
| **Raspberry Pi 3B+** | ARM64/ARMv7 | `dddns-linux-arm64` or `arm` | ✅ Full Support | ~12MB |
| **Raspberry Pi 3** | ARMv7 (32-bit) | `dddns-linux-arm` | ✅ Full Support | ~10MB |
| **Raspberry Pi 2** | ARMv7 (32-bit) | `dddns-linux-arm` | ✅ Full Support | ~10MB |
| **Raspberry Pi Zero 2 W** | ARM64 | `dddns-linux-arm64` | ✅ Full Support | ~12MB |
| **Raspberry Pi Zero W** | ARMv6 | `dddns-linux-arm` (v7) | ⚠️ May work | ~10MB |

## Installation Methods

### Method 1: Direct Binary Download (Recommended)

```bash
# For Raspberry Pi 4/5 with 64-bit OS (Raspberry Pi OS 64-bit)
wget https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_arm64.tar.gz
tar -xzf dddns_Linux_arm64.tar.gz
sudo mv dddns /usr/local/bin/
sudo chmod +x /usr/local/bin/dddns

# For Raspberry Pi with 32-bit OS
wget https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_armv7.tar.gz
tar -xzf dddns_Linux_armv7.tar.gz
sudo mv dddns /usr/local/bin/
sudo chmod +x /usr/local/bin/dddns
```

### Method 2: Package Manager (DEB)

```bash
# Download the appropriate .deb package
# For 64-bit OS (recommended for Pi 4/5)
wget https://github.com/descoped/dddns/releases/latest/download/dddns_arm64.deb
sudo dpkg -i dddns_arm64.deb

# For 32-bit OS
wget https://github.com/descoped/dddns/releases/latest/download/dddns_armhf.deb
sudo dpkg -i dddns_armhf.deb
```

### Method 3: Build from Source

```bash
# Install Go (if not already installed)
wget https://go.dev/dl/go1.21.5.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-arm64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build dddns
git clone https://github.com/descoped/dddns.git
cd dddns
make build
sudo make install
```

### Method 4: Install Script

```bash
#!/bin/bash
# install-dddns-pi.sh

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    aarch64|arm64)
        BINARY="dddns_Linux_arm64.tar.gz"
        ;;
    armv7l)
        BINARY="dddns_Linux_armv7.tar.gz"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Download and install
wget "https://github.com/descoped/dddns/releases/latest/download/$BINARY"
tar -xzf "$BINARY"
sudo mv dddns /usr/local/bin/
sudo chmod +x /usr/local/bin/dddns

# Create config directory
mkdir -p ~/.dddns

echo "✓ dddns installed successfully"
echo "Next: Run 'dddns config init' to set up configuration"
```

## Configuration

### Location

On Raspberry Pi, configuration is stored in the user's home directory:

```bash
~/.dddns/config.yaml     # Plain text config
~/.dddns/config.secure   # Encrypted config
~/.dddns/last-ip.txt     # IP cache
```

### Sample Configuration

```yaml
# ~/.dddns/config.yaml
aws_region: "us-east-1"
hosted_zone_id: "ZXXXXXXXXXXXXX"
hostname: "home.example.com"
ttl: 300
aws_access_key: "AKIAXXXXXXXXXXXXXX"
aws_secret_key: "xxxxxxxxxxxxxxxxxxxxxxxx"

# Pi-specific optimizations
ip_cache_file: "/home/pi/.dddns/last-ip.txt"
skip_proxy_check: false
```

## Setting Up Automatic Updates

### Option 1: Crontab (Recommended)

```bash
# Edit crontab
crontab -e

# Add this line for updates every 30 minutes
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1

# Or every 5 minutes for more frequent updates
*/5 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
```

### Option 2: Systemd Service

Create `/etc/systemd/system/dddns.service`:

```ini
[Unit]
Description=Dynamic DNS Updater
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=pi
ExecStart=/usr/local/bin/dddns update --quiet
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/dddns.timer`:

```ini
[Unit]
Description=Run dddns every 30 minutes
Requires=dddns.service

[Timer]
OnBootSec=5min
OnUnitActiveSec=30min

[Install]
WantedBy=timers.target
```

Enable the timer:

```bash
sudo systemctl daemon-reload
sudo systemctl enable dddns.timer
sudo systemctl start dddns.timer
sudo systemctl status dddns.timer
```

### Option 3: Network Event Hook

For immediate updates when network changes:

```bash
# Create dhcpcd exit hook
sudo nano /lib/dhcpcd/dhcpcd-hooks/90-dddns

#!/bin/bash
# Trigger dddns on interface changes
case "$reason" in
    BOUND|RENEW|REBIND|REBOOT|STATIC)
        if [ "$interface" = "eth0" ] || [ "$interface" = "wlan0" ]; then
            /usr/local/bin/dddns update --quiet &
        fi
        ;;
esac

# Make executable
sudo chmod +x /lib/dhcpcd/dhcpcd-hooks/90-dddns
```

## Performance Optimization

### Memory Usage

dddns is optimized for low memory usage:

```bash
# Check memory usage
ps aux | grep dddns

# Typical usage:
# USER  PID  %CPU %MEM    VSZ   RSS TTY STAT START TIME COMMAND
# pi    1234  0.0  0.3  710924 12288 ?   Sl   10:00 0:00 /usr/local/bin/dddns update
```

### SD Card Optimization

Minimize SD card writes:

```yaml
# config.yaml
# Use RAM disk for cache file
ip_cache_file: "/run/user/1000/dddns-last-ip.txt"

# Or disable cache (will update every run)
ip_cache_file: ""
```

### CPU Usage

dddns uses minimal CPU:
- Startup: ~100ms burst
- Runtime: <1% CPU
- Memory: 10-12MB resident

## Raspberry Pi Specific Features

### GPIO Integration (Future)

Potential for GPIO LED status indicators:

```go
// Future feature: LED status
// Green LED: DNS up to date
// Red LED: Update failed
// Blinking: Updating
```

### Temperature Monitoring (Future)

Could add Pi temperature to DNS TXT records:

```bash
# Get Pi temperature
vcgencmd measure_temp
# temp=42.0'C
```

## Use Cases

### 1. Home Lab Access

```bash
# Access your Pi home lab from anywhere
ssh pi@home.example.com
```

### 2. Self-Hosted Services

- **Nextcloud**: `cloud.home.example.com`
- **Home Assistant**: `ha.home.example.com`
- **Pi-hole**: `dns.home.example.com`
- **Jellyfin**: `media.home.example.com`

### 3. IoT Gateway

Use Pi as IoT gateway with dynamic DNS for remote access to smart home devices.

### 4. VPN Server

Run WireGuard/OpenVPN on Pi with dynamic DNS for stable endpoint.

## Troubleshooting

### Common Issues

1. **Architecture Mismatch**
```bash
# Check your architecture
uname -m

# For 64-bit OS on Pi 4/5
aarch64 or arm64 → use arm64 binary

# For 32-bit OS
armv7l → use armv7 binary
```

2. **Permission Denied**
```bash
# Fix permissions
sudo chmod +x /usr/local/bin/dddns
sudo chown pi:pi ~/.dddns/config.yaml
chmod 600 ~/.dddns/config.yaml
```

3. **Network Not Ready**
```bash
# Add delay to cron
@reboot sleep 60 && /usr/local/bin/dddns update --quiet
```

4. **SD Card Corruption**
```bash
# Use RAM disk for cache
ln -sf /run/user/$(id -u)/dddns-last-ip.txt ~/.dddns/last-ip.txt
```

### Debug Commands

```bash
# Test configuration
dddns config check

# Manual update with verbose output
dddns update

# Check current public IP
dddns ip

# Verify DNS record
dddns verify

# Check logs
journalctl -u dddns -f  # If using systemd
tail -f /var/log/dddns.log  # If using cron
```

## Resource Comparison

| Solution | Binary Size | Memory Usage | CPU Usage | Dependencies |
|----------|------------|--------------|-----------|--------------|
| **dddns** | 8MB | 10-12MB | <1% | None |
| ddclient | N/A (Perl) | 40-50MB | 2-5% | Perl + modules |
| inadyn | 200KB | 5-8MB | <1% | OpenSSL |
| python-route53 | N/A | 50-80MB | 5-10% | Python + boto3 |

## Security Considerations

### 1. Credential Storage

```bash
# Use secure config
dddns secure enable

# Set proper permissions
chmod 600 ~/.dddns/config.secure
```

### 2. Network Security

```bash
# Firewall rules (ufw)
sudo ufw allow from any to any port 443 proto tcp  # HTTPS for API calls
sudo ufw enable
```

### 3. User Isolation

```bash
# Run as non-root user
sudo useradd -r -s /bin/false dddns
sudo chown -R dddns:dddns /home/dddns/.dddns
```

## Power Failure Recovery

For Pi deployments with unstable power:

```bash
# Add to /etc/rc.local
sleep 30
/usr/local/bin/dddns update --force --quiet &
```

## Monitoring

### Health Check Script

```bash
#!/bin/bash
# /usr/local/bin/dddns-health.sh

# Check if DNS matches current IP
CURRENT_IP=$(curl -s https://checkip.amazonaws.com)
DNS_IP=$(dig +short home.example.com)

if [ "$CURRENT_IP" != "$DNS_IP" ]; then
    echo "DNS mismatch detected, forcing update"
    /usr/local/bin/dddns update --force
fi
```

### Integration with Pi Monitoring Tools

- **RPi-Monitor**: Add dddns status
- **Netdata**: Custom chart for DNS updates
- **Telegraf**: Metrics collection
- **Prometheus**: Export metrics

## Future Enhancements for Pi

1. **HAT Display Support**: Show IP/status on OLED
2. **GPIO LED Status**: Visual update indicators
3. **Power Optimization**: Reduce wake frequency
4. **Cluster Support**: Multi-Pi DNS failover
5. **mDNS Integration**: Local network discovery

## Conclusion

dddns is perfectly suited for Raspberry Pi deployments:

- **Lightweight**: 8MB binary, 12MB RAM
- **Efficient**: Minimal CPU and I/O
- **Reliable**: Survives reboots and network changes
- **Secure**: Encrypted credential storage
- **Simple**: No dependencies or complex setup

Whether running a single Pi or a cluster, dddns provides reliable dynamic DNS with minimal resource impact.