# Quick Start Guide

Get dddns up and running in 5 minutes!

## Prerequisites

Before you begin, you'll need:

1. **AWS Account** with Route53 hosted zone (see [AWS Setup Guide](aws-setup.md))
2. **Domain name** managed by Route53
3. **AWS credentials** with Route53 permissions
4. **Root access** to your device (for UDM) or sudo access (for Linux/macOS)

## Step 1: Install dddns

### For Ubiquiti Dream Machines (UDM / UDR / UDR7)

```bash
# One-line installation — the installer prompts for run mode (cron or serve)
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh)
```

The installer runs three safety gates (pre-flight, state snapshot, post-install smoke) and rolls back automatically on any failure. See the [Installation Guide](installation.md) for all flags.

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

## Step 2: Gather AWS Credentials

You'll need a Route53 access key pair scoped to a single hosted zone. Follow the [AWS Setup Guide](aws-setup.md) for the IAM policy; keep the access key ID and secret access key on hand for Step 3.

dddns does **not** read AWS environment variables or shared credential profiles — all credentials live in the dddns config file and are encryptable at rest with `dddns secure enable`.

## Step 3: Create Configuration

```bash
# Initialize configuration
dddns config init

# Edit configuration
vi ~/.dddns/config.yaml  # or /data/.dddns/config.yaml on UDM
```

Update the configuration with your settings:

```yaml
# AWS Settings (REQUIRED - no environment variables for security)
aws_region: "us-east-1"
aws_access_key: "AKIAIOSFODNN7EXAMPLE"
aws_secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

# DNS Settings (required)
hosted_zone_id: "Z1234567890ABC"    # Your Route53 Hosted Zone ID
hostname: "home.example.com"        # Domain to update
ttl: 300
```

**Important**: The config file must have `0600` permissions. dddns refuses to load it otherwise:

```bash
chmod 600 ~/.dddns/config.yaml         # Linux / macOS
chmod 600 /data/.dddns/config.yaml     # UDM / UDR
```

### Finding Your Hosted Zone ID

```bash
# List all hosted zones (uses your default AWS CLI credentials, separate from dddns)
aws route53 list-hosted-zones

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

The installer sets up the mode you selected:

- **Cron mode** — `/etc/cron.d/dddns` runs `dddns update --quiet` every 30 minutes. Log file only grows when the IP actually changes or something fails.
- **Serve mode** — `/etc/systemd/system/dddns.service` hosts a loopback listener; UniFi's `inadyn` pushes to it on every WAN IP change. See below for the 30-second setup.

### Serve Mode on UniFi (30 seconds)

If you ran the installer with `--mode serve` (or picked `2) serve` at the prompt), it already printed your UniFi UI values and the shared secret. If you picked cron and want to switch later:

```bash
# 1. Initialise the serve-mode config block and print the shared secret once
dddns config rotate-secret --init

# 2. Switch the boot script to serve mode and apply
dddns config set-mode serve
sudo /data/on_boot.d/20-dddns.sh

# 3. Paste the secret into UniFi UI → Settings → Internet → Dynamic DNS:
#      Service:  Custom
#      Hostname: home.example.com          (must match cfg.Hostname)
#      Username: dddns                     (handler ignores this field)
#      Password: <the secret from step 1>
#      Server:   127.0.0.1:53353/nic/update?hostname=%h&myip=%i
```

Verify with `dddns serve test` and `dddns serve status`. Full walkthrough in the [UDM Guide](udm-guide.md).

### For Linux/macOS

```bash
# Add to crontab
crontab -e

# Add this line:
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
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

- Read the [Configuration Guide](configuration.md) for advanced settings
- Check [Troubleshooting](troubleshooting.md) if you encounter issues
- For UDM users, see the [UDM Guide](udm-guide.md) for device-specific information

## Need Help?

If you run into issues:

1. Check the logs: `tail -f /var/log/dddns.log` (cron) or `journalctl -u dddns -f` (serve)
2. Run with dry-run: `dddns update --dry-run`
3. Validate config: `dddns config check`
4. On UniFi, run the privacy-safe probe: `bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) --probe`
5. See [Troubleshooting Guide](troubleshooting.md)
6. For AWS setup help, see [AWS Setup Guide](aws-setup.md)