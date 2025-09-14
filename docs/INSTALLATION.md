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

| Model | Full Name | OS Version | Processor | Architecture | Tested |
|-------|-----------|------------|-----------|--------------|--------|
| UDM | Dream Machine | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ARM64 (RISC) | ❌ |
| UDM-Pro | Dream Machine Pro | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ARM64 (RISC) | ❌ |
| UDM-SE | Dream Machine SE | UniFi OS 2.x/3.x | ARM Cortex-A57 (1.7 GHz, 4-core) | ARM64 (RISC) | ❌ |
| UDM Pro Max | Dream Machine Pro Max | UniFi OS 3.x/4.x | ARM Cortex-A57 (2.0 GHz, 4-core) | ARM64 (RISC) | ❌ |
| UDR | Dream Router | UniFi OS 3.x/4.x | ARM (1.35 GHz, 2-core) | ARM64 (RISC) | ❌ |
| UDR7 | Dream Router 7 | UniFi OS 4.x | ARM (Wi-Fi 7 capable) | ARM64 (RISC) | ✅ |

> **⚠️ Important Disclaimer**: Only the UDR7 (Dream Router 7) has been fully tested. While other models should work due to similar architecture, users should proceed with caution and verify compatibility on their specific device.

> **📦 Boot Persistence Dependency**: All UniFi devices require [unifios-utilities](https://github.com/unifi-utilities/unifios-utilities) or manual `/data/on_boot.d/` setup for persistence across firmware updates. The installer will prompt to install this if not present.

### Automated Installation (Recommended)

```bash
# Download and run installer
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
```

The installer will:
- ✅ Check environment compatibility
- ✅ Install on-boot-script (if needed)
- ✅ Download the ARM64 binary
- ✅ Set up persistent boot scripts
- ✅ Configure cron for automatic updates
- ✅ Create default configuration

### Installation Options

```bash
# Check environment only (no installation)
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | \
  bash -s -- --check-only

# Install specific version
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | \
  bash -s -- --version v1.0.0

# Force reinstall/upgrade
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | \
  bash -s -- --force

# Uninstall
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | \
  bash -s -- --uninstall
```

### Manual Installation

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

# 4. Create boot script
cat > /data/on_boot.d/20-dddns.sh << 'EOF'
#!/bin/bash
ln -sf /data/dddns/dddns /usr/local/bin/dddns
cat > /etc/cron.d/dddns << 'CRON'
*/30 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
CRON
/etc/init.d/cron restart
EOF
chmod +x /data/on_boot.d/20-dddns.sh

# 5. Run boot script
/data/on_boot.d/20-dddns.sh

# 6. Initialize configuration
dddns config init
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
2. Run `make update-dddns VERSION=vX.Y.Z`
3. Commit and push the updated formula

## Building from Source

### Prerequisites

- Go 1.21 or later
- Make
- Git

### Build Steps

```bash
# Clone repository
git clone https://github.com/descoped/dddns.git
cd dddns

# Build for current platform
make build

# Build for UDM
make build-udm

# Build for all platforms
make build-all

# Install locally
sudo make install

# Run tests
make test
```

### Cross-Compilation

```bash
# Build for UDM (ARM64)
GOOS=linux GOARCH=arm64 go build -o dddns-linux-arm64 .

# Build for Linux AMD64
GOOS=linux GOARCH=amd64 go build -o dddns-linux-amd64 .

# Build for macOS Intel
GOOS=darwin GOARCH=amd64 go build -o dddns-darwin-amd64 .

# Build for macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dddns-darwin-arm64 .
```

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
# Re-run installer with --force
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash -s -- --force
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
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash -s -- --uninstall
```

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

- [Configure dddns](CONFIGURATION.md)
- [Command Reference](COMMANDS.md)
- [Troubleshooting](TROUBLESHOOTING.md)