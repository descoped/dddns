# Troubleshooting Guide

This guide helps you diagnose and fix common issues with dddns.

## Table of Contents
- [Quick Diagnostics](#quick-diagnostics)
- [Installation Issues](#installation-issues)
- [Configuration Problems](#configuration-problems)
- [AWS/Route53 Errors](#awsroute53-errors)
- [Network Issues](#network-issues)
- [UDM-Specific Issues](#udm-specific-issues)
- [Serve Mode Issues](#serve-mode-issues)
- [Update Not Working](#update-not-working)
- [Debug Mode](#debug-mode)
- [Common Error Messages](#common-error-messages)

## Quick Diagnostics

Run these commands to quickly identify issues:

```bash
# Always applicable
dddns --version                 # is the binary on PATH?
dddns config check              # is config present and valid?
dddns ip                        # is the public-IP lookup working?

# Cron mode
dddns update --dry-run          # exercise the full update flow
tail -50 /var/log/dddns.log     # what did the last cron run say?

# Serve mode
pgrep -laf "dddns serve"        # is the daemon running?
dddns serve status              # when did it last handle a request?
dddns serve test                # can we reach it from this shell?
tail -n 20 /var/log/dddns-audit.log | jq -c '{ts,auth,action,error}'
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
DEBUG=1 ./install-on-unifi-os.sh

# See the available flags
./install-on-unifi-os.sh --help

# Install specific mode non-interactively (skips the prompt)
./install-on-unifi-os.sh --mode cron
./install-on-unifi-os.sh --mode serve

# Force re-install over the existing version
./install-on-unifi-os.sh --force
```

The installer verifies the binary's SHA-256 against the release's `checksums.txt` before extracting. If it aborts with `SHA-256 mismatch` or `Could not fetch checksums.txt`, treat that as a hard failure — do not bypass it. Check you're on the latest release tag and that your network isn't MITM-ing GitHub.

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

1. The supported IAM policy is the condition-key-scoped one in the [AWS Setup Guide → Creating IAM User](aws-setup.md#step-3-create-the-scoped-policy). With that policy the record name in the condition block must match exactly what dddns is trying to upsert — Route53 normalises to lowercase with no trailing dot, so make sure the policy and `cfg.Hostname` agree on that form.

2. Only `UPSERT` is permitted by the scoped policy. dddns never issues `DELETE` or `CREATE` on its own, so this only surfaces if a zone-wide policy is in place and something else is attempting those actions.

3. Verify the hosted zone ID:
   ```bash
   aws route53 list-hosted-zones --profile dddns
   grep hosted_zone_id /data/.dddns/config.yaml
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
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash

# Verify files still exist
ls -la /data/dddns/
ls -la /data/.dddns/
```

### Cron Not Running

**Symptom**: Updates not happening automatically (cron mode only)

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

If you're in serve mode, there is no cron entry — `/etc/cron.d/dddns` should be absent. Updates are triggered by the UniFi Dynamic DNS UI; see [Serve Mode Issues](#serve-mode-issues) below.

## Serve Mode Issues

`dddns serve` returns responses in the standard dyndns v2 protocol: plain text, trailing newline, HTTP 200 (except for network-origin / wrong-method rejections which are raw HTTP 403 / 405). The body's first token is the diagnostic — this section walks the common ones.

### Confirming the listener is alive

Start here when anything in serve mode looks wrong:

```bash
# Is the daemon running?
pgrep -laf "dddns serve"

# When did it last handle a request?
dddns serve status

# Can we reach it from the router?
dddns serve test

# Tail both log files in parallel
tail -f /var/log/dddns-server.log /var/log/dddns-audit.log
```

If `pgrep` returns nothing, the supervised loop is not running. Re-apply the boot script:

```bash
sudo /data/on_boot.d/20-dddns.sh
```

### `badauth`

**Symptom**: UniFi UI shows the DDNS entry in an error state; `dddns serve test` returns `Body: badauth`; audit log shows `"auth":"bad"` or `"auth":"missing"`.

**Cause**: the Password field in the UniFi Dynamic DNS dialog does not match the shared secret stored in `cfg.Server.SharedSecret`. Either rotated out of sync or never set correctly.

**Fix**:

```bash
# Generate a new secret and print it once
dddns config rotate-secret

# Copy the printed value into:
#   UniFi Network Controller → Settings → Internet →
#   Dynamic DNS → <your entry> → Password
```

If the audit log shows `"auth":"locked"` instead, lockout is active — see below.

### `badauth` that looks like lockout

**Symptom**: Every request returns `badauth` even right after rotating the secret and updating the UniFi UI.

**Cause**: The listener trips the sliding-window lockout after 5 failed auth attempts within 60 seconds. Once tripped, every subsequent request is rejected with `badauth` (the response is deliberately indistinguishable from a normal auth failure — don't give attackers extra signal) for 5 minutes.

**How to tell them apart**: check the audit log for `"auth":"locked"`:

```bash
grep '"auth":"locked"' /var/log/dddns-audit.log | tail -5
```

**Fix**: wait out the 5-minute window, or restart the daemon to reset the in-memory lockout state:

```bash
pkill -f "dddns serve"
# Supervised loop restarts after 5 seconds automatically.
```

### `nohost`

**Symptom**: `dddns serve test` returns `Body: nohost`; audit log records `"action":"nohost"`.

**Cause**: the `hostname` query parameter from `inadyn` does not match `cfg.Hostname`. Either the UniFi UI's Hostname field is different from config, or the hostname was changed in one place and not the other.

**Fix**:

```bash
# What does config say?
grep -E '^\s*hostname:' /data/.dddns/config.yaml

# What does the UniFi UI send?
# Open the Dynamic DNS entry and compare — they must match exactly,
# including subdomain and zone.
```

### `notfqdn`

**Symptom**: `Body: notfqdn`.

**Cause**: the `hostname` query parameter is missing entirely, or the `myip` parameter did not parse as a valid IPv4 address. Typically a misconfigured `Server` field in UniFi's UI that omits the `%h` or `%i` tokens.

**Fix**: the Server field must be exactly:

```
127.0.0.1:53353/nic/update?hostname=%h&myip=%i
```

### `dnserr`

**Symptom**: `Body: dnserr`; audit log has a populated `error` field.

**Cause**: upstream failure — either the WAN interface lookup failed (no public IPv4 found on the detected interface) or the Route53 API call failed (credentials, connectivity, throttling, or a condition-key rejection from the scoped IAM policy).

**Fix**: read the audit entry's `error` field — it carries the underlying message:

```bash
tail -n 20 /var/log/dddns-audit.log | jq -r '{ts,action,error}'
```

Common causes and next steps:

- **Interface has no public IP** — verify with `ip -4 addr show` on the WAN interface; ensure you're not behind CGNAT (100.64.0.0/10 addresses are rejected).
- **Route53 AccessDenied** — the scoped IAM policy requires the record name to match exactly (normalised lowercase, no trailing dot) and action to be `UPSERT`. See the [AWS Setup Guide → AccessDenied](aws-setup.md#accessdenied-error).
- **Route53 throttled** — the handler respects its 30-second context deadline; on throttling, retry after a minute. Inadyn will also retry on its own schedule.

### `911`

**Symptom**: `Body: 911`.

**Cause**: the handler recovered from a panic. This is a bug — please collect context and file an issue.

**Fix**:

```bash
# Grab the daemon log for the panic stack
tail -n 200 /var/log/dddns-server.log

# Capture the audit entry for the offending request
grep '"action":"panic"' /var/log/dddns-audit.log | tail -1

# Restart to clear state
pkill -f "dddns serve"
```

### HTTP 403 (not in allowed_cidrs)

**Symptom**: `curl` from a remote host returns HTTP 403 with an empty body; audit log shows `"action":"cidr-deny"`.

**Cause**: the `RemoteAddr` of the incoming connection is not in `cfg.Server.AllowedCIDRs`. This is the fail-closed L1 defense working correctly — an external attacker should see exactly this.

**Fix (legitimate case)**: if you explicitly want LAN reachability (e.g. you moved the bind to `0.0.0.0:53353` for debugging), widen the allowlist:

```yaml
server:
  bind: "0.0.0.0:53353"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "192.168.1.0/24"   # your LAN subnet, NOT the whole RFC1918 space
```

Then `dddns config set-mode serve` to rewrite the boot script and `sudo /data/on_boot.d/20-dddns.sh` to apply.

Keep the default loopback-only bind if the only caller is UniFi's on-device `inadyn` — that's the security default, and loosening it is an opt-in risk.

### `dddns serve test` says `connection refused`

**Symptom**: Can't reach `127.0.0.1:53353` even from the router itself.

**Cause**: the daemon is not running. Supervisor may have crashed; cron mode might be active instead; or the port is something other than the default.

**Fix**:

```bash
# Confirm mode
grep "^# --- " /data/on_boot.d/20-dddns.sh

# Confirm the bind port
grep '^\s*bind:' /data/.dddns/config.yaml

# Restart
sudo /data/on_boot.d/20-dddns.sh
```

### Mode switching didn't take effect

**Symptom**: Ran `dddns config set-mode serve` but the cron entry is still there, or vice versa.

**Cause**: `set-mode` only rewrites the boot script — it does not run it. The old mode's artefacts persist until the boot script is executed (on reboot or manually).

**Fix**:

```bash
sudo /data/on_boot.d/20-dddns.sh
```

Or reboot. The generated script is idempotent — re-running it fully switches between modes (removing `/etc/cron.d/dddns` on serve mode; `pkill`-ing any stray serve loop on cron mode).

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