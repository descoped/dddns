#!/bin/bash
#
# dddns Installation Script for Ubiquiti Dream Machines
#
# One-line installation:
#   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh | bash
#
# Or download and run:
#   ./install.sh [--uninstall] [--force]
#

set -e

# Configuration
readonly GITHUB_REPO="descoped/dddns"
readonly INSTALL_DIR="/data/dddns"
readonly BINARY_NAME="dddns"
readonly CONFIG_DIR="/data/.dddns"
readonly BOOT_SCRIPT_DIR="/data/on_boot.d"
readonly BOOT_SCRIPT_NAME="20-dddns.sh"
readonly CRON_FILE="/etc/cron.d/dddns"
readonly LOG_FILE="/var/log/dddns.log"

# Colors
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1" >&2; }
log_warning() { echo -e "${YELLOW}[!]${NC} $1"; }

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

# Detect architecture
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        aarch64|arm64)
            ARCH="arm64"
            log_success "Detected ARM64 architecture"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            log_error "UDM devices require ARM64"
            exit 1
            ;;
    esac
}

# Check if this is a UDM/UDR device
check_udm() {
    # First check if /data exists (required for all UniFi devices)
    if [[ ! -d "/data" ]]; then
        log_error "/data directory not found - this doesn't appear to be a UDM/UDR device"
        exit 1
    fi

    # Detect UniFi OS version
    if [[ -f /etc/unifi-os/unifi-os.conf ]]; then
        log_info "Detected UniFi OS v3 (UDM)"
    elif [[ -d /etc/unifi-core ]] || [[ -f /etc/default/unifi ]]; then
        log_info "Detected UniFi OS v4 (UDM/UDR)"
    elif [[ -f /etc/board.info ]]; then
        log_info "Detected Ubiquiti device (via board.info)"
    elif [[ -d /data/unifi ]]; then
        log_info "Detected Ubiquiti device (via /data/unifi)"
    else
        log_warning "Could not determine UniFi OS version, but /data exists - continuing"
    fi

    # Check available space
    local available=$(df -BM /data | awk 'NR==2 {print $4}' | sed 's/M//')
    if [[ $available -lt 50 ]]; then
        log_warning "Low disk space: ${available}MB available (50MB recommended)"
    else
        log_success "Disk space: ${available}MB available"
    fi
}

# Install unifios-utilities if needed
install_unifios_utilities() {
    if [[ ! -f "/data/on_boot.sh" ]] && [[ ! -d "/data/on_boot.d" ]]; then
        log_warning "Boot persistence requires unifios-utilities or on_boot.d support"
        log_info "This ensures dddns survives firmware updates"
        echo ""
        echo -n "Install unifios-utilities for boot persistence? [Y/n]: "
        read -r response </dev/tty || response="y"

        if [[ -z "$response" ]] || [[ "$response" =~ ^[Yy] ]]; then
            log_info "Installing unifios-utilities..."
            if curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash; then
                log_success "unifios-utilities installed"
            else
                log_error "Failed to install unifios-utilities"
                log_info "You may need to install it manually for persistence across reboots"
            fi
        else
            log_warning "Skipping unifios-utilities installation"
            log_info "Note: dddns may not persist after firmware updates without it"
        fi
    else
        if [[ -d "/data/on_boot.d" ]]; then
            log_success "Boot persistence directory already exists"
        else
            log_success "unifios-utilities already installed"
        fi
    fi

    # Ensure boot script directory exists
    mkdir -p "${BOOT_SCRIPT_DIR}"
}

# Get latest release version
get_latest_version() {
    # Don't log here as it interferes with command substitution
    local version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
                    grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [[ -z "$version" ]]; then
        log_error "Failed to get latest version from GitHub"
        exit 1
    fi

    echo "$version"
}

# Download and install binary
install_binary() {
    local version="$1"
    local force="${2:-false}"

    # Check if already installed
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]] && [[ "$force" != "true" ]]; then
        local current=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $3}')
        if [[ "$current" == "$version" ]] || [[ "v$current" == "$version" ]]; then
            log_success "dddns ${version} already installed"
            return 0
        fi
    fi

    log_info "Downloading dddns ${version}..."

    # Create installation directory
    mkdir -p "${INSTALL_DIR}"

    # Download archive (GoReleaser format)
    local archive_name="dddns_Linux_${ARCH}.tar.gz"
    if [[ "${ARCH}" == "arm" ]]; then
        archive_name="dddns_Linux_armv7.tar.gz"
    fi

    local url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${archive_name}"
    local temp_archive="/tmp/dddns-${version}.tar.gz"
    local temp_dir="/tmp/dddns-${version}-extract"

    log_info "Downloading from: ${url}"

    if curl -L -o "${temp_archive}" "$url" --progress-bar; then
        log_info "Extracting archive..."

        # Create temp directory and extract
        mkdir -p "${temp_dir}"
        if tar -xzf "${temp_archive}" -C "${temp_dir}"; then
            # Find and move the binary
            if [[ -f "${temp_dir}/${BINARY_NAME}" ]]; then
                chmod +x "${temp_dir}/${BINARY_NAME}"
                mv "${temp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
                log_success "Binary installed successfully"

                # Cleanup
                rm -rf "${temp_dir}" "${temp_archive}"
            else
                log_error "Binary not found in archive"
                ls -la "${temp_dir}"
                rm -rf "${temp_dir}" "${temp_archive}"
                exit 1
            fi
        else
            log_error "Failed to extract archive"
            rm -f "${temp_archive}"
            exit 1
        fi
    else
        log_error "Failed to download from ${url}"
        exit 1
    fi

    # Create symlink
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
}

# Create boot persistence script
create_boot_script() {
    log_info "Creating boot persistence script..."

    cat > "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}" << 'EOF'
#!/bin/bash
#
# dddns Boot Persistence Script
# Ensures dddns survives reboots and firmware updates
#

BINARY_PATH="/data/dddns/dddns"
CONFIG_DIR="/data/.dddns"
LOG_FILE="/var/log/dddns.log"

# Create symlink if needed
if [[ -f "$BINARY_PATH" ]] && [[ ! -L "/usr/local/bin/dddns" ]]; then
    ln -sf "$BINARY_PATH" "/usr/local/bin/dddns"
fi

# Ensure config directory exists
if [[ ! -d "$CONFIG_DIR" ]]; then
    mkdir -p "$CONFIG_DIR"
    chmod 700 "$CONFIG_DIR"
fi

# Create/update cron job
cat > /etc/cron.d/dddns << 'CRON'
# dddns - Dynamic DNS updater for Route53
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Update DNS every 30 minutes (with timestamp for visibility)
*/30 * * * * root echo "[$(date '+\%Y-\%m-\%d \%H:\%M:\%S')] Running dddns update..." >> /var/log/dddns.log && /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
CRON

# Restart cron
/etc/init.d/cron restart >/dev/null 2>&1

# Log rotation (keep log under 10MB)
if [[ -f "$LOG_FILE" ]]; then
    SIZE=$(stat -c%s "$LOG_FILE" 2>/dev/null || echo 0)
    if [[ $SIZE -gt 10485760 ]]; then
        mv "$LOG_FILE" "$LOG_FILE.old"
        touch "$LOG_FILE"
    fi
fi

echo "[$(date)] dddns boot script completed" >> /var/log/dddns-boot.log
EOF

    chmod +x "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    log_success "Boot script created"
}

# Create default configuration
create_default_config() {
    # Check if any configuration exists (upgrade scenario)
    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        log_info "Existing configuration detected - preserving user settings"
        return 0
    fi

    log_info "Creating default configuration..."
    mkdir -p "${CONFIG_DIR}"
    chmod 700 "${CONFIG_DIR}"

    cat > "${CONFIG_DIR}/config.yaml" << 'EOF'
# dddns Configuration
#
# Update with your AWS credentials and Route53 settings
# For secure credentials, use: dddns secure --init

aws:
  region: "us-east-1"
  # Option 1: Use AWS CLI profile (if AWS CLI is installed)
  # profile: "your-profile-name"

  # Option 2: Direct credentials (less secure)
  # access_key_id: "YOUR_ACCESS_KEY"
  # secret_access_key: "YOUR_SECRET_KEY"

dns:
  hosted_zone_id: "YOUR_HOSTED_ZONE_ID"  # e.g., "Z1234567890ABC"
  hostname: "your.domain.com"            # Domain to update
  ttl: 300                                # Time-to-live in seconds

operations:
  ip_cache_file: "/data/.dddns/last-ip.txt"
  skip_proxy_check: false                # Set true if behind VPN
EOF

    chmod 600 "${CONFIG_DIR}/config.yaml"
    log_warning "Default configuration created at ${CONFIG_DIR}/config.yaml"
    log_warning "Please edit it with your AWS credentials and DNS settings"
}

# Setup cron job
setup_cron() {
    log_info "Setting up cron job..."

    cat > "${CRON_FILE}" << 'EOF'
# dddns - Dynamic DNS updater for Route53
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Update DNS every 30 minutes (with timestamp for visibility)
*/30 * * * * root echo "[$(date '+%Y-%m-%d %H:%M:%S')] Running dddns update..." >> /var/log/dddns.log && /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
EOF

    # Restart cron
    /etc/init.d/cron restart >/dev/null 2>&1
    log_success "Cron job configured"
}

# Uninstall function
uninstall() {
    log_warning "Uninstalling dddns..."

    # Remove cron job
    rm -f "${CRON_FILE}"
    /etc/init.d/cron restart >/dev/null 2>&1

    # Remove boot script
    rm -f "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"

    # Remove symlink
    rm -f "/usr/local/bin/${BINARY_NAME}"

    # Remove binary
    rm -rf "${INSTALL_DIR}"

    log_warning "Configuration preserved at ${CONFIG_DIR}"
    log_info "To remove configuration: rm -rf ${CONFIG_DIR}"
    log_success "dddns uninstalled"
}

# Main installation
main() {
    local action="install"
    local force="false"

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --uninstall)
                action="uninstall"
                shift
                ;;
            --force)
                force="true"
                shift
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --force      Force reinstall"
                echo "  --uninstall  Remove dddns"
                echo "  --help       Show this help"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    # Header
    echo ""
    echo "======================================"
    echo "  dddns Installer for UDM"
    echo "======================================"
    echo ""

    # Check requirements
    check_root
    detect_arch
    check_udm

    # Handle uninstall
    if [[ "$action" == "uninstall" ]]; then
        uninstall
        exit 0
    fi

    # Detect if this is an upgrade
    local is_upgrade=false
    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        is_upgrade=true
    fi

    # Show installation plan and get confirmation (unless --force)
    if [[ "$force" != "true" ]]; then
        echo ""
        if [[ "$is_upgrade" == "true" ]]; then
            log_info "Upgrade detected - existing configuration will be preserved"
            log_info "Upgrade Plan:"
            echo "  • Update dddns binary in: /data/dddns/"
            echo "  • Update symlink at: /usr/local/bin/dddns"
            echo "  • Update boot persistence script"
            echo "  • Preserve existing configuration"
        else
            log_info "Installation Plan:"
            echo "  • Install dddns binary to: /data/dddns/"
            echo "  • Create symlink at: /usr/local/bin/dddns"
            echo "  • Set up boot persistence in: /data/on_boot.d/"
            echo "  • Configure cron job to run every 30 minutes"
            echo "  • Create config directory at: /data/.dddns/"
            echo "  • Log output to: /var/log/dddns.log"
        fi
        echo ""
        echo -n "Proceed with installation? [Y/n]: "
        read -r response </dev/tty || response="y"

        if [[ "$response" =~ ^[Nn] ]]; then
            log_info "Installation cancelled by user"
            exit 0
        fi
        echo ""
    fi

    # Install unifios-utilities if needed
    install_unifios_utilities

    # Get version and install
    log_info "Fetching latest version..."
    version=$(get_latest_version)
    log_info "Latest version: $version"
    install_binary "$version" "$force"

    # Setup persistence and configuration
    create_boot_script
    create_default_config
    setup_cron

    # Run boot script to apply immediately
    log_info "Applying configuration..."
    "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"

    # Test installation
    echo ""
    if "${INSTALL_DIR}/${BINARY_NAME}" --version &>/dev/null; then
        local installed=$("${INSTALL_DIR}/${BINARY_NAME}" --version)
        log_success "Installation complete: $installed"
    else
        log_error "Installation test failed"
        exit 1
    fi

    # Final instructions
    echo ""
    echo "======================================"
    if [[ "$is_upgrade" == "true" ]]; then
        echo "  Upgrade Complete!"
    else
        echo "  Installation Complete!"
    fi
    echo "======================================"
    echo ""

    if [[ "$is_upgrade" == "true" ]]; then
        echo "Your existing configuration has been preserved."
        echo ""
        echo "Next steps:"
        echo "1. Test: dddns update --dry-run"
        echo "2. Monitor: tail -f ${LOG_FILE}"
    else
        echo "Next steps:"
        echo "1. Edit configuration: vi ${CONFIG_DIR}/config.yaml"
        echo "2. Add your AWS credentials and Route53 settings"
        echo "3. Test: dddns update --dry-run"
        echo "4. Monitor: tail -f ${LOG_FILE}"
    fi
    echo ""
    echo "The cron job will run every 30 minutes automatically."
    echo "Logs are rotated automatically when they exceed 10MB."
    echo ""
}

# Run main
main "$@"