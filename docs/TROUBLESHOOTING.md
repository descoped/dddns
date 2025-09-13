# Troubleshooting Guide

This guide helps you diagnose and fix common issues with dddns.

## Table of Contents
- [Quick Diagnostics](#quick-diagnostics)
- [Installation Issues](#installation-issues)
- [Configuration Problems](#configuration-problems)
- [AWS/Route53 Errors](#awsroute53-errors)
- [Network Issues](#network-issues)
- [UDM-Specific Issues](#udm-specific-issues)
- [Update Not Working](#update-not-working)
- [Debug Mode](#debug-mode)
- [Common Error Messages](#common-error-messages)

## Quick Diagnostics

Run these commands to quickly identify issues:

```bash
# 1. Check if dddns is installed
dddns --version

# 2. Validate configuration
dddns config check

# 3. Test IP resolution
dddns ip

# 4. Test update without making changes
dddns update --dry-run

# 5. Check logs (location varies by platform)
tail -50 /var/log/dddns.log           # Linux/UDM
tail -50 /tmp/dddns.log               # macOS
journalctl -u dddns -n 50             # systemd
```

## Installation Issues

### Command Not Found

**Symptom**: `bash: dddns: command not found`

**Solutions**:

```bash
# Check if binary exists
ls -la /usr/local/bin/dddns

# Recreate symlink (UDM)
ln -sf /data/dddns/dddns /usr/local/bin/dddns

# Add to PATH (Linux/macOS)
export PATH=$PATH:/usr/local/bin
echo 'export PATH=$PATH:/usr/local/bin' >> ~/.bashrc
```

### Permission Denied

**Symptom**: `Permission denied` when running dddns

**Solutions**:

```bash
# Make binary executable
chmod +x /usr/local/bin/dddns

# Check file ownership
ls -la /usr/local/bin/dddns

# Run with sudo if needed
sudo dddns update
```

### Installation Script Fails

**Symptom**: Installer exits with error

**Solutions**:

```bash
# Run with debug mode
DEBUG=1 ./install-dddns-udm.sh

# Check environment first
./install-dddns-udm.sh --check-only

# Manual installation
mkdir -p /data/dddns
curl -L -o /data/dddns/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-linux-arm64
chmod +x /data/dddns/dddns
```

## Configuration Problems

### Config File Not Found

**Symptom**: `Error: configuration file not found`

**Solutions**:

```bash
# Create default config
dddns config init

# Check config locations
ls -la ~/.dddns/config.yaml        # Linux/macOS
ls -la /data/.dddns/config.yaml    # UDM

# Specify config path
dddns update --config /path/to/config.yaml
```

### Invalid Configuration

**Symptom**: `Error: invalid configuration: hosted_zone_id is required`

**Solutions**:

```bash
# Check config syntax
cat ~/.dddns/config.yaml

# Validate YAML
yamllint ~/.dddns/config.yaml

# Example valid config
cat > ~/.dddns/config.yaml << EOF
aws_profile: "default"
aws_region: "us-east-1"
hosted_zone_id: "Z1234567890ABC"
hostname: "home.example.com"
ttl: 300
EOF
```

### Config Permission Issues

**Symptom**: `Error: config file has incorrect permissions`

**Solutions**:

```bash
# Fix permissions
chmod 600 ~/.dddns/config.yaml
chmod 700 ~/.dddns

# Check ownership
chown $(whoami):$(whoami) ~/.dddns/config.yaml
```

## AWS/Route53 Errors

### AWS Credentials Not Found

**Symptom**: `Error: failed to load AWS config`

**Solutions**:

```bash
# Check AWS profile
aws configure list --profile dddns

# Set up credentials
aws configure --profile dddns

# Use environment variables
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret
export AWS_REGION=us-east-1

# Check credentials file
cat ~/.aws/credentials
```

### Access Denied

**Symptom**: `AccessDenied: User is not authorized to perform: route53:ChangeResourceRecordSets`

**Solutions**:

1. Check IAM permissions:
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

2. Verify hosted zone ID:
```bash
aws route53 list-hosted-zones --profile dddns
```

### Hosted Zone Not Found

**Symptom**: `Error: hosted zone not found`

**Solutions**:

```bash
# List all hosted zones
aws route53 list-hosted-zones --profile dddns \
  --query "HostedZones[*].[Id,Name]" --output table

# Verify zone ID in config
grep hosted_zone_id ~/.dddns/config.yaml

# Update config with correct ID
vi ~/.dddns/config.yaml
```

### Record Not Found

**Symptom**: `Error: A record not found for hostname`

**Solutions**:

```bash
# Check if record exists
aws route53 list-resource-record-sets \
  --hosted-zone-id YOUR_ZONE_ID \
  --query "ResourceRecordSets[?Name=='home.example.com.']"

# Create record first time
dddns update --force
```

## Network Issues

### Cannot Reach AWS

**Symptom**: `Error: failed to get public IP: network error`

**Solutions**:

```bash
# Test connectivity
ping -c 1 checkip.amazonaws.com
curl -s https://checkip.amazonaws.com

# Check DNS resolution
nslookup checkip.amazonaws.com
host route53.amazonaws.com

# Test with different DNS
echo "nameserver 8.8.8.8" > /etc/resolv.conf
```

### Proxy/VPN Detected

**Symptom**: `Error: proxy/VPN detected for IP, skipping update`

**Solutions**:

```bash
# Skip proxy check if behind corporate proxy
# In config.yaml:
skip_proxy_check: true

# Or via command line
dddns update --skip-proxy

# Check if actually using proxy
curl -s http://ip-api.com/json/ | jq .proxy
```

### Timeout Errors

**Symptom**: `Error: context deadline exceeded`

**Solutions**:

```bash
# Increase timeout (if configurable in future versions)
# For now, check network latency
ping -c 10 route53.amazonaws.com

# Try again during off-peak hours
# Check for network issues
traceroute route53.amazonaws.com
```

## UDM-Specific Issues

### Lost After Reboot

**Symptom**: dddns not available after UDM reboot

**Solutions**:

```bash
# Check boot script
ls -la /data/on_boot.d/20-dddns.sh

# Run boot script manually
/data/on_boot.d/20-dddns.sh

# Verify on-boot-script installed
ls -la /data/on_boot.sh

# Reinstall if needed
curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash
```

### Firmware Update Issues

**Symptom**: dddns stops working after UniFi OS update

**Solutions**:

```bash
# Reinstall on-boot-script
curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash

# Re-run dddns installer
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-dddns-udm.sh | bash

# Verify files still exist
ls -la /data/dddns/
ls -la /data/.dddns/
```

### Cron Not Running

**Symptom**: Updates not happening automatically

**Solutions**:

```bash
# Check cron service
service cron status

# Restart cron
/etc/init.d/cron restart

# Verify cron job exists
cat /etc/cron.d/dddns

# Check cron logs
grep CRON /var/log/messages | tail -20

# Recreate cron job
/data/on_boot.d/20-dddns.sh
```

## Update Not Working

### IP Not Updating

**Symptom**: DNS record not updating despite IP change

**Solutions**:

```bash
# Check cached IP
cat /tmp/dddns-last-ip.txt      # or /data/.dddns/last-ip.txt

# Force update
dddns update --force

# Remove cache file
rm /tmp/dddns-last-ip.txt
dddns update

# Verify DNS propagation
dig +short home.example.com @8.8.8.8
nslookup home.example.com
```

### Dry Run Works, Actual Update Fails

**Symptom**: `--dry-run` succeeds but real update fails

**Solutions**:

```bash
# Check AWS credentials have write permissions
aws route53 change-resource-record-sets \
  --hosted-zone-id YOUR_ZONE_ID \
  --change-batch file://test-change.json

# Enable debug mode
DEBUG=1 dddns update

# Check for rate limiting
# Wait 5 minutes and try again
```

## Debug Mode

### Enable Verbose Output

```bash
# Set DEBUG environment variable
DEBUG=1 dddns update

# For persistent debugging (UDM)
echo 'DEBUG=1' >> /data/.dddns/.env
```

### Check All Components

```bash
#!/bin/bash
# Debug script

echo "=== Environment ==="
uname -a
echo

echo "=== dddns Version ==="
dddns --version
echo

echo "=== Configuration ==="
dddns config check
echo

echo "=== Network ==="
curl -s https://checkip.amazonaws.com
echo

echo "=== AWS Access ==="
aws route53 list-hosted-zones --profile dddns --query "HostedZones[0].Id"
echo

echo "=== Current DNS ==="
HOSTNAME=$(grep hostname ~/.dddns/config.yaml | cut -d: -f2 | tr -d ' "')
dig +short $HOSTNAME
echo

echo "=== Cached IP ==="
cat /tmp/dddns-last-ip.txt 2>/dev/null || echo "No cache"
echo

echo "=== Test Update ==="
dddns update --dry-run
```

## Common Error Messages

### "Failed to get public IP"

**Cause**: Cannot reach checkip.amazonaws.com

**Fix**:
```bash
# Test alternative services
curl -s https://api.ipify.org
curl -s https://ifconfig.me

# Check firewall rules
iptables -L -n | grep 443
```

### "Invalid configuration"

**Cause**: Missing required fields in config

**Fix**:
```bash
# Regenerate config
mv ~/.dddns/config.yaml ~/.dddns/config.yaml.bak
dddns config init
# Copy values from backup
```

### "Context deadline exceeded"

**Cause**: Network timeout

**Fix**:
```bash
# Check network latency
ping -c 10 route53.amazonaws.com

# Check DNS resolver
cat /etc/resolv.conf
```

### "No such host"

**Cause**: DNS resolution failure

**Fix**:
```bash
# Add public DNS
echo "nameserver 1.1.1.1" >> /etc/resolv.conf
echo "nameserver 8.8.8.8" >> /etc/resolv.conf
```

## Getting Additional Help

If these solutions don't resolve your issue:

1. **Collect debug information**:
   ```bash
   dddns --version > debug.log
   dddns config check >> debug.log 2>&1
   DEBUG=1 dddns update --dry-run >> debug.log 2>&1
   tail -100 /var/log/dddns.log >> debug.log
   ```

2. **Check GitHub Issues**:
   - Search existing issues: https://github.com/descoped/dddns/issues
   - Create new issue with debug.log attached

3. **Community Support**:
   - GitHub Discussions: https://github.com/descoped/dddns/discussions
   - Include your device model, UniFi OS version, and error messages

## Prevention Tips

1. **Regular Testing**:
   ```bash
   # Weekly test
   dddns update --dry-run
   ```

2. **Monitor Logs**:
   ```bash
   # Check for errors weekly
   grep -i error /var/log/dddns.log
   ```

3. **Backup Configuration**:
   ```bash
   cp -r ~/.dddns ~/.dddns.backup
   ```

4. **Keep Updated**:
   ```bash
   # Check for updates
   curl -s https://api.github.com/repos/descoped/dddns/releases/latest | grep tag_name
   ```