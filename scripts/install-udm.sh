#!/bin/bash
#
# dddns Installation and Update Script for Ubiquiti Dream Machines
# Supports: UDM, UDM-Pro, UDM-SE, UDM Pro Max, UDR, UDR7
#
# This script installs dddns CLI to persistent storage and configures
# it to survive reboots and firmware updates on UniFi OS devices.
#
# Usage:
#   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-udm.sh | bash
#   or
#   ./install-udm.sh [--version v1.0.0] [--force] [--uninstall]
#

set -e

# Configuration
GITHUB_REPO="descoped/dddns"
INSTALL_DIR="/data/dddns"
BINARY_NAME="dddns"
CONFIG_DIR="/data/.dddns"
BOOT_SCRIPT_DIR="/data/on_boot.d"
BOOT_SCRIPT_NAME="10-dddns.sh"
CRON_FILE="/etc/cron.d/dddns"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

# Detect UniFi device type and architecture
detect_device() {
    local device_info=$(ubnt-device-info firmware || echo "unknown")
    local uname_info=$(uname -m)
    
    log_info "Detected architecture: $uname_info"
    log_info "Device info: $device_info"
    
    # All current UniFi Dream devices use ARM64
    if [[ "$uname_info" == "aarch64" ]] || [[ "$uname_info" == "arm64" ]]; then
        ARCH="arm64"
        log_success "Detected ARM64 architecture"
    else
        log_error "Unsupported architecture: $uname_info"
        exit 1
    fi
    
    # Detect UniFi OS version
    if [ -f "/etc/unifi-os/unifi-os.conf" ]; then
        UNIFI_OS_VERSION=$(grep "UNIFI_OS_VERSION" /etc/unifi-os/unifi-os.conf | cut -d'=' -f2)
        log_info "UniFi OS Version: $UNIFI_OS_VERSION"
    fi
}

# Get latest release version from GitHub
get_latest_version() {
    local latest_version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$latest_version" ]; then
        log_error "Failed to get latest version from GitHub"
        exit 1
    fi
    
    echo "$latest_version"
}

# Download and install binary
install_binary() {
    local version=$1
    local force=$2
    
    # Check if already installed
    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ] && [ "$force" != "true" ]; then
        local current_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
        log_info "Current version: $current_version"
        
        if [ "$current_version" == "$version" ]; then
            log_success "dddns $version is already installed"
            return 0
        fi
    fi
    
    log_info "Installing dddns version: $version"
    
    # Create installation directory
    mkdir -p "${INSTALL_DIR}"
    
    # Download binary
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/dddns-linux-${ARCH}"
    log_info "Downloading from: $download_url"
    
    if ! curl -L -o "${INSTALL_DIR}/${BINARY_NAME}.tmp" "$download_url"; then
        log_error "Failed to download binary"
        exit 1
    fi
    
    # Make executable and move to final location
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}.tmp"
    mv "${INSTALL_DIR}/${BINARY_NAME}.tmp" "${INSTALL_DIR}/${BINARY_NAME}"
    
    # Create symlink in PATH
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
    
    log_success "Binary installed successfully"
}

# Create boot script for persistence
create_boot_script() {
    log_info "Creating boot persistence script"
    
    # Ensure on-boot.d directory exists
    mkdir -p "${BOOT_SCRIPT_DIR}"
    
    cat > "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}" << 'EOF'
#!/bin/bash
#
# dddns Boot Script
# Ensures dddns is available after reboot
#

# Create symlink if it doesn't exist
if [ ! -L "/usr/local/bin/dddns" ]; then
    ln -sf /data/dddns/dddns /usr/local/bin/dddns
fi

# Ensure config directory exists with correct permissions
if [ ! -d "/data/.dddns" ]; then
    mkdir -p /data/.dddns
    chmod 700 /data/.dddns
fi

# Re-create cron job
cat > /etc/cron.d/dddns << 'CRON'
# dddns - Dynamic DNS updater for Route53
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Run every 30 minutes
*/30 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
CRON

# Restart cron to pick up changes
/etc/init.d/cron restart

echo "[$(date)] dddns boot script completed" >> /var/log/dddns-boot.log
EOF
    
    chmod +x "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    
    log_success "Boot script created"
}

# Install on-boot-script if not present
install_on_boot_script() {
    if [ ! -f "/data/on_boot.sh" ]; then
        log_info "Installing unifios-utilities on-boot-script"
        curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash
        log_success "on-boot-script installed"
    else
        log_info "on-boot-script already installed"
    fi
}

# Setup configuration
setup_config() {
    log_info "Setting up configuration"
    
    # Create config directory
    mkdir -p "${CONFIG_DIR}"
    chmod 700 "${CONFIG_DIR}"
    
    # Check if config exists
    if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
        log_info "Creating default configuration"
        
        cat > "${CONFIG_DIR}/config.yaml" << 'EOF'
# dddns Configuration
# Please update with your AWS and DNS settings

aws:
  profile: "route66dns"
  region: "us-east-1"
  credential_source: "profile"

dns:
  hosted_zone_id: "YOUR_HOSTED_ZONE_ID"
  hostname: "your-domain.com"
  record_type: "A"
  ttl: 300

operations:
  receipt_file: "/data/.dddns/last-ip.txt"
  log_file: "/var/log/dddns.log"
  check_interval: "30m"
  network_timeout: "10s"
  skip_proxy_check: false
  force_update: false
  require_root: true

services:
  ip_check_url: "https://checkip.amazonaws.com"
  proxy_check_url: "http://ip-api.com/json/"
EOF
        
        log_warning "Default configuration created at ${CONFIG_DIR}/config.yaml"
        log_warning "Please update it with your AWS credentials and DNS settings"
    else
        log_info "Configuration already exists"
    fi
}

# Setup cron job
setup_cron() {
    log_info "Setting up cron job"
    
    cat > "${CRON_FILE}" << 'EOF'
# dddns - Dynamic DNS updater for Route53
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Run every 30 minutes
*/30 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
EOF
    
    # Restart cron
    /etc/init.d/cron restart
    
    log_success "Cron job configured"
}

# Create update script
create_update_script() {
    log_info "Creating update script"
    
    cat > "${INSTALL_DIR}/update.sh" << 'EOF'
#!/bin/bash
#
# dddns Update Script
# Updates dddns to the latest version
#

echo "Checking for dddns updates..."
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-udm.sh | bash -s -- --force
EOF
    
    chmod +x "${INSTALL_DIR}/update.sh"
    ln -sf "${INSTALL_DIR}/update.sh" "/usr/local/bin/dddns-update"
    
    log_success "Update script created (run 'dddns-update' to update)"
}

# Uninstall function
uninstall() {
    log_warning "Uninstalling dddns..."
    
    # Stop cron job
    rm -f "${CRON_FILE}"
    
    # Remove boot script
    rm -f "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    
    # Remove symlinks
    rm -f "/usr/local/bin/${BINARY_NAME}"
    rm -f "/usr/local/bin/dddns-update"
    
    # Remove binary
    rm -rf "${INSTALL_DIR}"
    
    log_warning "Configuration preserved at ${CONFIG_DIR}"
    log_warning "To remove configuration: rm -rf ${CONFIG_DIR}"
    log_success "dddns uninstalled"
}

# Test installation
test_installation() {
    log_info "Testing installation..."
    
    # Test binary
    if ! "${INSTALL_DIR}/${BINARY_NAME}" --version; then
        log_error "Binary test failed"
        return 1
    fi
    
    # Test configuration
    if ! "${INSTALL_DIR}/${BINARY_NAME}" config validate; then
        log_warning "Configuration validation failed - please update your config"
    fi
    
    log_success "Installation test completed"
}

# Main installation flow
main() {
    local version=""
    local force="false"
    local action="install"
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                version="$2"
                shift 2
                ;;
            --force)
                force="true"
                shift
                ;;
            --uninstall)
                action="uninstall"
                shift
                ;;
            --help)
                echo "Usage: $0 [--version VERSION] [--force] [--uninstall]"
                echo ""
                echo "Options:"
                echo "  --version VERSION  Install specific version (default: latest)"
                echo "  --force           Force reinstall even if already installed"
                echo "  --uninstall       Remove dddns installation"
                echo "  --help            Show this help message"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Check root
    check_root
    
    # Detect device
    detect_device
    
    # Handle uninstall
    if [ "$action" == "uninstall" ]; then
        uninstall
        exit 0
    fi
    
    # Get version if not specified
    if [ -z "$version" ]; then
        version=$(get_latest_version)
        log_info "Latest version: $version"
    fi
    
    # Installation steps
    install_on_boot_script
    install_binary "$version" "$force"
    create_boot_script
    setup_config
    setup_cron
    create_update_script
    
    # Run boot script to apply changes
    "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    
    # Test installation
    test_installation
    
    echo ""
    log_success "==================================="
    log_success "dddns installation completed!"
    log_success "==================================="
    echo ""
    echo "Next steps:"
    echo "1. Edit configuration: vi ${CONFIG_DIR}/config.yaml"
    echo "2. Test manually: dddns update --dry-run"
    echo "3. Check logs: tail -f /var/log/dddns.log"
    echo ""
    echo "To update dddns: dddns-update"
    echo "To uninstall: curl -fsL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-udm.sh | bash -s -- --uninstall"
    echo ""
}

# Run main function
main "$@"