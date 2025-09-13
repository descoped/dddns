# Quick Start Guide

Get dddns up and running in 5 minutes!

## Prerequisites

Before you begin, you'll need:

1. **AWS Account** with Route53 hosted zone
2. **Domain name** managed by Route53
3. **AWS credentials** with Route53 permissions
4. **Root access** to your device (for UDM) or sudo access (for Linux/macOS)

## Step 1: Install dddns

### For Ubiquiti Dream Machines (UDM/UDR)

```bash
# One-line installation
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-dddns-udm.sh | bash
```

### For Linux/macOS

```bash
# Download latest release
curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-$(uname -s)-$(uname -m)

# Make executable
chmod +x /usr/local/bin/dddns

# Create config directory
mkdir -p ~/.dddns
```

## Step 2: Configure AWS Credentials

```bash
# Configure AWS CLI profile
aws configure --profile dddns

# Enter when prompted:
AWS Access Key ID: YOUR_ACCESS_KEY
AWS Secret Access Key: YOUR_SECRET_KEY
Default region name: us-east-1
Default output format: json
```

This creates a secure credentials file at `~/.aws/credentials` with restricted permissions.

## Step 3: Create Configuration

```bash
# Initialize configuration
dddns config init

# Edit configuration
vi ~/.dddns/config.yaml  # or /data/.dddns/config.yaml on UDM
```

Update the configuration with your settings:

```yaml
# AWS Settings (REQUIRED - single source of truth)
aws_profile: "dddns"     # References ~/.aws/credentials profile
aws_region: "us-east-1"

# DNS Settings (required)
hosted_zone_id: "Z1234567890ABC"    # Your Route53 Hosted Zone ID
hostname: "home.example.com"        # Domain to update
ttl: 300

# Operational Settings
ip_cache_file: "/tmp/dddns-last-ip.txt"
skip_proxy_check: false
```

**Important**: The config file must have restricted permissions (600):
```bash
chmod 600 ~/.dddns/config.yaml
```

### Finding Your Hosted Zone ID

```bash
# List all hosted zones
aws route53 list-hosted-zones --profile dddns

# Or in AWS Console:
# Route53 → Hosted zones → Select your domain → Copy Hosted zone ID
```

## Step 4: Test Configuration

```bash
# Validate configuration
dddns config check

# Check current public IP
dddns ip

# Test update (dry run - no changes)
dddns update --dry-run
```

## Step 5: Run First Update

```bash
# Perform actual update
dddns update
```

Expected output:
```
[2024-01-15 10:30:00] Checking for IP changes...
Current public IP: 203.0.113.42
Last known IP: 203.0.113.41
Current DNS record: 203.0.113.41
Updating home.example.com to 203.0.113.42...
Successfully updated home.example.com to 203.0.113.42
```

## Step 6: Set Up Automatic Updates

### For UDM (Already Done)

The installer automatically sets up a cron job to run every 30 minutes.

### For Linux/macOS

```bash
# Add to crontab
crontab -e

# Add this line:
*/30 * * * * /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
```

## Verify It's Working

```bash
# Check DNS propagation
dig +short home.example.com

# Monitor logs
tail -f /var/log/dddns.log

# Force an update
dddns update --force
```

## Common Commands

```bash
# Show current IP
dddns ip

# Update DNS (dry run)
dddns update --dry-run

# Update DNS
dddns update

# Force update (ignore cache)
dddns update --force

# Check configuration
dddns config check

# Show version
dddns --version
```

## What's Next?

- Read the [Configuration Guide](CONFIGURATION.md) for advanced settings
- Check [Troubleshooting](TROUBLESHOOTING.md) if you encounter issues
- For UDM users, see the [UDM Guide](UDM_GUIDE.md) for device-specific information

## Need Help?

If you run into issues:

1. Check the logs: `tail -f /var/log/dddns.log`
2. Run with dry-run: `dddns update --dry-run`
3. Verify AWS credentials: `aws route53 list-hosted-zones --profile dddns`
4. See [Troubleshooting Guide](TROUBLESHOOTING.md)