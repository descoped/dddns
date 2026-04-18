# Installation Guide

This guide covers installation methods for all supported platforms.

## Table of Contents
- [Ubiquiti Dream Machine](#ubiquiti-dream-machine)
- [Linux](#linux)
- [macOS](#macos)
- [Docker](#docker)
- [Building from Source](#building-from-source)

## Ubiquiti Dream Machine

### Supported Models

| Model | Full Name | OS Version | Processor | Tested |
|-------|-----------|------------|-----------|--------|
| UDM | Dream Machine | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ❌ |
| UDM-Pro | Dream Machine Pro | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ❌ |
| UDM-SE | Dream Machine SE | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ❌ |
| UDM Pro Max | Dream Machine Pro Max | UniFi OS 3.x/4.x | ARM Cortex-A57 (2.0 GHz, 4-core) | ❌ |
| UDR | Dream Router | UniFi OS 3.x/4.x | ARM (1.35 GHz, 2-core) | ❌ |
| UDR7 | Dream Router 7 | UniFi OS 4.x | ARM (Wi-Fi 7 capable) | ✅ |

**Architecture**: All models use ARM64 (aarch64) 64-bit processors with Little Endian byte ordering.

> **⚠️ Important Disclaimer**: Only the UDR7 (Dream Router 7) has been fully tested. While other models should work due to similar architecture, users should proceed with caution and verify compatibility on their specific device.

> **📦 Boot Persistence Dependency**: All UniFi devices require [unifios-utilities](https://github.com/unifi-utilities/unifios-utilities) or manual `/data/on_boot.d/` setup for persistence across firmware updates. The installer will prompt to install this if not present.

### Automated Installation (Recommended)

```bash
# Download and run installer
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh)
```

The installer will:
- Check environment compatibility (device, arch, /data persistence, systemd, disk space)
- Install `unifios-utilities` on-boot-script hook if absent
- Download the ARM64 binary and verify its SHA-256 against `checksums.txt`
- Prompt for run mode (cron or serve) unless `--mode` is passed
- Generate `/data/on_boot.d/20-dddns.sh` via `dddns config set-mode`
- Install `/etc/cron.d/dddns` (cron mode) or `/etc/systemd/system/dddns.service` (serve mode)
- Create a default `/data/.dddns/config.yaml` with 0600 permissions if none exists

### Installer Flags

```bash
# Install a specific release (required for pre-releases — GitHub's "latest" excludes RCs)
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --version v0.2.0

# Pick a mode non-interactively
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --mode serve

# Verbose — show all subprocess output (systemctl, cron restart, boot script)
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --verbose

# Privacy-safe self-diagnosis — prints device, arch, disk, cron, systemd,
# and install metadata with no WAN IPs, no config values, no log contents.
# Safe to paste in a GitHub issue. Changes no state.
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --probe

# Roll back to the previous binary + boot script + cron/systemd entry
# from the .prev snapshots written by the last install
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --rollback

# Uninstall (preserves /data/.dddns so reinstalling keeps your config)
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) \
  --uninstall
```

`DDDNS_DEBUG=1` has the same effect as `--verbose`. `DDDNS_VERSION` is equivalent to `--version`.

### Safety Gates

Every install and upgrade runs through three gates. Any failure reverts or refuses the install with the previous version left intact.

1. **Pre-flight.** Downloads the new binary to a temp dir, runs `--version` and `config check` against the **existing** config *before* touching any live file. If the new binary rejects the current config, the running install is untouched.
2. **State snapshot.** The prior binary, boot script, cron entry, and systemd unit are copied to `*.prev` siblings. `--rollback` restores them in one shot.
3. **Post-install smoke.** After the boot script has applied the mode, the now-live binary re-runs `--version` and `config check`. If either fails, the installer auto-rolls back to the `.prev` state and exits non-zero.

Combined, the gates make upgrades safe to run from cron/ansible without manual approval — a broken release cannot cause downtime.

### Release Verification

The installer downloads `checksums.txt` from the GitHub release alongside the tarball and verifies the binary's SHA-256 before extracting. A mismatch aborts the install with `SHA-256 mismatch` or `Could not fetch checksums.txt` — treat these as hard failures, not noise.

### Manual Installation

The automated installer handles boot-script generation, SHA-256 verification, and mode switching. Manual installs skip those safety nets — prefer `install-on-unifi-os.sh`. If you still want to do it by hand:

```bash
# 1. Create directories
mkdir -p /data/dddns
mkdir -p /data/.dddns
mkdir -p /data/on_boot.d

# 2. Download and extract binary
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_arm64.tar.gz | \
  tar -xz -C /data/dddns dddns
chmod +x /data/dddns/dddns

# 3. Create symlink
ln -sf /data/dddns/dddns /usr/local/bin/dddns

# 4. Initialize configuration (creates /data/.dddns/config.yaml with 0600 perms)
dddns config init

# 5. Generate the boot script via the binary itself (don't hand-write it —
#    set-mode emits the canonical, idempotent version with mode-switching logic).
dddns config set-mode cron    # or: dddns config set-mode serve

# 6. Apply immediately (set-mode only writes the file; it doesn't run it)
sudo /data/on_boot.d/20-dddns.sh
```

## Linux

### Package Installation

#### Debian/Ubuntu (.deb)
```bash
# AMD64/x86_64
curl -LO https://github.com/descoped/dddns/releases/latest/download/dddns_linux_amd64.deb
sudo dpkg -i dddns_linux_amd64.deb

# ARM64/aarch64
curl -LO https://github.com/descoped/dddns/releases/latest/download/dddns_linux_arm64.deb
sudo dpkg -i dddns_linux_arm64.deb
```

#### Red Hat/CentOS/Fedora (.rpm)
```bash
# AMD64/x86_64
sudo rpm -ivh https://github.com/descoped/dddns/releases/latest/download/dddns_linux_amd64.rpm

# ARM64/aarch64
sudo rpm -ivh https://github.com/descoped/dddns/releases/latest/download/dddns_linux_arm64.rpm

# For Fedora/RHEL 8+ (using dnf):
sudo dnf install https://github.com/descoped/dddns/releases/latest/download/dddns_linux_amd64.rpm
```

#### Alpine Linux (.apk)
```bash
# AMD64/x86_64
wget https://github.com/descoped/dddns/releases/latest/download/dddns_linux_amd64.apk
sudo apk add --allow-untrusted dddns_linux_amd64.apk

# ARM64/aarch64
wget https://github.com/descoped/dddns/releases/latest/download/dddns_linux_arm64.apk
sudo apk add --allow-untrusted dddns_linux_arm64.apk
```

### Binary Installation

```bash
# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="x86_64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="armv7" ;;
esac

# Download and extract binary
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_${ARCH}.tar.gz | \
  sudo tar -xz -C /usr/local/bin dddns

# Make executable
sudo chmod +x /usr/local/bin/dddns

# Create config directory
mkdir -p ~/.dddns

# Initialize configuration
dddns config init
```

### Systemd Service

```bash
# Create service file
sudo tee /etc/systemd/system/dddns.timer << EOF
[Unit]
Description=Run dddns every 30 minutes
Requires=dddns.service

[Timer]
OnCalendar=*:0/30
Persistent=true

[Install]
WantedBy=timers.target
EOF

sudo tee /etc/systemd/system/dddns.service << EOF
[Unit]
Description=Update Route53 DNS with public IP
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
ExecStart=/usr/local/bin/dddns update
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Enable and start timer
sudo systemctl daemon-reload
sudo systemctl enable dddns.timer
sudo systemctl start dddns.timer

# Check status
sudo systemctl status dddns.timer
sudo journalctl -u dddns -f
```

## macOS

### Homebrew

```bash
# Tap the repository
brew tap descoped/tap
brew install dddns

# Update to latest version
brew upgrade dddns

# Uninstall
brew uninstall dddns
```

> **Note**: The Homebrew formula is maintained in the [homebrew-tap](https://github.com/descoped/homebrew-tap) repository.

### Binary Installation

```bash
# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="x86_64" ;;
    arm64) ARCH="arm64" ;;
esac

# Download and extract binary
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Darwin_${ARCH}.tar.gz | \
  sudo tar -xz -C /usr/local/bin dddns

# Make executable
sudo chmod +x /usr/local/bin/dddns

# Create config directory
mkdir -p ~/.dddns

# Initialize configuration
dddns config init
```

### LaunchAgent (Auto-run)

```bash
# Create LaunchAgent
tee ~/Library/LaunchAgents/com.dddns.updater.plist << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.dddns.updater</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/dddns</string>
        <string>update</string>
    </array>
    <key>StartInterval</key>
    <integer>1800</integer>
    <key>StandardOutPath</key>
    <string>/tmp/dddns.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/dddns.error.log</string>
</dict>
</plist>
EOF

# Load LaunchAgent
launchctl load ~/Library/LaunchAgents/com.dddns.updater.plist

# Check status
launchctl list | grep dddns
```

## Docker

### Docker Run

```bash
# Create config directory
mkdir -p ~/.dddns

# Create config file first
dddns config init

# Run with Docker
docker run -d \
  --name dddns \
  --restart unless-stopped \
  -v ~/.dddns:/data/.dddns:ro \
  -v ~/.aws:/root/.aws:ro \
  ghcr.io/descoped/dddns:latest \
  update
```

### Docker Compose

```yaml
version: '3'
services:
  dddns:
    image: ghcr.io/descoped/dddns:latest
    container_name: dddns
    restart: unless-stopped
    volumes:
      - ~/.dddns:/data/.dddns:ro
      - ~/.aws:/root/.aws:ro
    command: update
    environment:
      - DDDNS_CHECK_INTERVAL=30m
```

## Homebrew Formula Maintenance

For maintainers: Homebrew formulas are maintained in the separate [homebrew-tap](https://github.com/descoped/homebrew-tap) repository.

After creating a new dddns release:
1. Go to the homebrew-tap repository
2. Run the formula update recipe defined in that repo (see its README)
3. Commit and push the updated formula

## Building from Source

### Prerequisites

- Go 1.26 or later
- [just](https://github.com/casey/just) (replaces the old Makefile)
- Git

### Build Steps

```bash
# Clone repository
git clone https://github.com/descoped/dddns.git
cd dddns

# Build for current platform
just build

# Install locally (copies bin/dddns to /usr/local/bin)
just install

# Run tests
just test
```

### Cross-Compilation

Release artefacts for every supported OS/arch are produced by **GoReleaser**
on every tag push (see `.github/workflows/goreleaser.yml`). For a one-off
local build of a specific target, use `go build` directly with the
standard `GOOS` / `GOARCH` env vars:

```bash
# UDM / UDR / Raspberry Pi (ARM64)
GOOS=linux GOARCH=arm64 go build -o dddns-linux-arm64 .

# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o dddns-linux-amd64 .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dddns-darwin-arm64 .

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o dddns-darwin-amd64 .
```

To exercise the full GoReleaser matrix locally, install GoReleaser and
run `goreleaser build --snapshot --clean`.

## Verification

After installation, verify everything is working:

```bash
# Check version
dddns --version

# Check configuration
dddns config check

# Test IP resolution
dddns ip

# Test update (dry run)
dddns update --dry-run
```

## Upgrading

### UDM
```bash
# Re-run installer — safe to re-run; preserves the current mode unless
# --mode is passed explicitly. Use --force only to reinstall the same
# version in place.
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh)

# Upgrade to a specific release (required for RC testing)
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) --version v0.2.0

# If an upgrade goes wrong, roll back to the previous snapshot
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) --rollback
```

### Linux
```bash
# For package installations (deb/rpm/apk)
# Re-download and install the latest package version

# For binary installations
ARCH=$(uname -m | sed 's/x86_64/x86_64/;s/aarch64/arm64/;s/armv7l/armv7/')
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_${ARCH}.tar.gz | \
  sudo tar -xz -C /usr/local/bin dddns
sudo chmod +x /usr/local/bin/dddns
```

### macOS
```bash
# Using Homebrew
brew upgrade dddns

# For binary installation
ARCH=$(uname -m | sed 's/x86_64/x86_64/')
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Darwin_${ARCH}.tar.gz | \
  sudo tar -xz -C /usr/local/bin dddns
sudo chmod +x /usr/local/bin/dddns
```

## Uninstalling

### UDM
```bash
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh) --uninstall
```
Configuration at `/data/.dddns/` is preserved. Remove it manually with `rm -rf /data/.dddns` if you want a full wipe.

### Linux
```bash
# Remove binary
sudo rm /usr/local/bin/dddns

# Remove configuration (optional)
rm -rf ~/.dddns

# Remove systemd service
sudo systemctl stop dddns.timer
sudo systemctl disable dddns.timer
sudo rm /etc/systemd/system/dddns.*
sudo systemctl daemon-reload
```

### macOS
```bash
# Using Homebrew
brew uninstall dddns

# For binary installation
sudo rm /usr/local/bin/dddns

# Remove configuration (optional)
rm -rf ~/.dddns

# Remove LaunchAgent if configured
launchctl unload ~/Library/LaunchAgents/com.dddns.updater.plist
rm ~/Library/LaunchAgents/com.dddns.updater.plist
```

## Next Steps

- [Configure dddns](configuration.md)
- [Command Reference](commands.md)
- [Troubleshooting](troubleshooting.md)