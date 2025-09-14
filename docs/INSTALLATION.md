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
- UDM (Dream Machine)
- UDM-Pro
- UDM-SE (Special Edition)
- UDM Pro Max
- UDR (Dream Router)
- UDR7 (Dream Router 7)

### Automated Installation (Recommended)

```bash
# Download and run installer
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash
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
curl -fsL [...]/install.sh | bash -s -- --check-only

# Install specific version
curl -fsL [...]/install.sh | bash -s -- --version v1.0.0

# Force reinstall/upgrade
curl -fsL [...]/install.sh | bash -s -- --force

# Uninstall
curl -fsL [...]/install.sh | bash -s -- --uninstall
```

### Manual Installation

```bash
# 1. Create directories
mkdir -p /data/dddns
mkdir -p /data/.dddns
mkdir -p /data/on_boot.d

# 2. Download binary
curl -L -o /data/dddns/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-linux-arm64
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
# Download the latest .deb package
curl -LO https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_x86_64.deb
# Or for ARM64: dddns_Linux_arm64.deb

# Install
sudo dpkg -i dddns_Linux_x86_64.deb

# Or in one command:
curl -L https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_x86_64.deb | sudo dpkg -i -
```

#### Red Hat/CentOS/Fedora (.rpm)
```bash
# Download and install the latest .rpm package
sudo rpm -ivh https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_x86_64.rpm
# Or for ARM64: dddns_Linux_arm64.rpm

# For Fedora/RHEL 8+ you can also use dnf:
sudo dnf install https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_x86_64.rpm
```

#### Alpine Linux (.apk)
```bash
# Download the latest .apk package
wget https://github.com/descoped/dddns/releases/latest/download/dddns_Linux_x86_64.apk
# Or for ARM64: dddns_Linux_arm64.apk

# Install
sudo apk add --allow-untrusted dddns_Linux_x86_64.apk
```

### Binary Installation

```bash
# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
esac

# Download binary
sudo curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-linux-${ARCH}

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
# Tap the repository and install
brew tap descoped/dddns
brew install dddns

# Update to latest version
brew upgrade dddns

# Uninstall
brew uninstall dddns
```

> **Note**: The Homebrew formula is automatically updated with each release via GoReleaser.

### Binary Installation

```bash
# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64) ARCH="arm64" ;;
esac

# Download binary
sudo curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-darwin-${ARCH}

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

For maintainers: After creating a new release, update the Homebrew formula:

```bash
# Update formula with new version (after GitHub release is published)
make update-formula VERSION=vX.Y.Z

# Commit and push the updated formula
git add Formula/dddns.rb
git commit -m "chore: update Formula to vX.Y.Z"
git push origin main
```

The formula is located at `Formula/dddns.rb` and contains checksums for macOS binaries only.

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

# Build for all platforms
make build-udm

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
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash -s -- --force
```

### Linux/macOS
```bash
# Download new binary
sudo curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-$(uname -s)-$(uname -m)
sudo chmod +x /usr/local/bin/dddns
```

## Uninstalling

### UDM
```bash
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash -s -- --uninstall
```

### Linux/macOS
```bash
# Remove binary
sudo rm /usr/local/bin/dddns

# Remove configuration (optional)
rm -rf ~/.dddns

# Remove systemd service (Linux)
sudo systemctl stop dddns.timer
sudo systemctl disable dddns.timer
sudo rm /etc/systemd/system/dddns.*

# Remove LaunchAgent (macOS)
launchctl unload ~/Library/LaunchAgents/com.dddns.updater.plist
rm ~/Library/LaunchAgents/com.dddns.updater.plist
```

## Next Steps

- [Configure dddns](CONFIGURATION.md)
- [Command Reference](COMMANDS.md)
- [Troubleshooting](TROUBLESHOOTING.md)