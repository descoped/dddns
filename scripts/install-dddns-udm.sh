#!/bin/bash
#
# dddns Installation Script for Ubiquiti Dream Machines
# Version: 2.0.0
#
# This script performs comprehensive environment checks and installs dddns
# on UDM/UDR devices with proper persistence across reboots and updates.
#
# Tested on: UDM, UDM-Pro, UDM-SE, UDM Pro Max, UDR, UDR7
#
# Usage:
#   ./install-dddns-udm.sh [--force] [--version VERSION] [--uninstall] [--check-only]
#

set -e

# ============================================================================
# Configuration
# ============================================================================

readonly SCRIPT_VERSION="2.0.0"
readonly GITHUB_REPO="descoped/dddns"
readonly INSTALL_DIR="/data/dddns"
readonly BINARY_NAME="dddns"
readonly CONFIG_DIR="/data/.dddns"
readonly BOOT_SCRIPT_DIR="/data/on_boot.d"
readonly BOOT_SCRIPT_NAME="20-dddns.sh"
readonly CRON_FILE="/etc/cron.d/dddns"
readonly LOG_FILE="/var/log/dddns.log"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m' # No Color

# ============================================================================
# Logging Functions
# ============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

log_debug() {
    if [[ "${DEBUG:-0}" == "1" ]]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

print_header() {
    echo ""
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  dddns Installer for Ubiquiti Dream Machines v${SCRIPT_VERSION}${NC}"
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ============================================================================
# Environment Detection Functions
# ============================================================================

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
    log_success "Running as root"
}

detect_architecture() {
    local arch=$(uname -m)
    case "$arch" in
        aarch64|arm64)
            ARCH="arm64"
            log_success "Architecture: ARM64 (${arch})"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            log_error "This installer only supports ARM64 UDM devices"
            exit 1
            ;;
    esac
}

detect_device_info() {
    log_info "Detecting device information..."
    
    # Get kernel info
    local kernel_info=$(uname -r)
    log_info "Kernel: ${kernel_info}"
    
    # Try to get device model
    local device_model="Unknown"
    if command -v ubnt-device-info &> /dev/null; then
        device_model=$(ubnt-device-info model 2>/dev/null || echo "Unknown")
    fi
    log_info "Device Model: ${device_model}"
    
    # Check for UniFi OS version
    if [[ -f "/etc/unifi-os/unifi-os.conf" ]]; then
        source /etc/unifi-os/unifi-os.conf
        log_info "UniFi OS Version: ${UNIFI_OS_VERSION:-Unknown}"
        UNIFI_OS_MAJOR=$(echo "${UNIFI_OS_VERSION:-0}" | cut -d. -f1)
    else
        log_warning "Cannot determine UniFi OS version"
        UNIFI_OS_MAJOR="2"  # Assume 2.x if can't detect
    fi
    
    # Check hostname
    log_info "Hostname: $(hostname)"
}

check_persistent_storage() {
    log_info "Checking persistent storage..."
    
    if [[ ! -d "/data" ]]; then
        log_error "/data directory not found - this doesn't appear to be a UDM device"
        exit 1
    fi
    
    # Check available space
    local available_space=$(df -BM /data | awk 'NR==2 {print $4}' | sed 's/M//')
    if [[ $available_space -lt 50 ]]; then
        log_warning "Low disk space on /data: ${available_space}MB available"
        log_warning "At least 50MB recommended for dddns installation"
    else
        log_success "/data has ${available_space}MB available"
    fi
    
    # Check if /data is writable
    if touch /data/.write_test 2>/dev/null; then
        rm -f /data/.write_test
        log_success "/data is writable"
    else
        log_error "/data is not writable"
        exit 1
    fi
}

check_on_boot_script() {
    log_info "Checking on-boot-script setup..."
    
    # Check if on_boot.d directory exists
    if [[ -d "${BOOT_SCRIPT_DIR}" ]]; then
        log_success "Boot script directory exists: ${BOOT_SCRIPT_DIR}"
        
        # List existing boot scripts
        local script_count=$(ls -1 ${BOOT_SCRIPT_DIR}/*.sh 2>/dev/null | wc -l)
        if [[ $script_count -gt 0 ]]; then
            log_info "Found ${script_count} existing boot script(s):"
            ls -la ${BOOT_SCRIPT_DIR}/*.sh | while read line; do
                log_debug "  $line"
            done
        fi
        
        ON_BOOT_EXISTS=true
    else
        log_warning "Boot script directory not found"
        ON_BOOT_EXISTS=false
    fi
    
    # Check if unifios-utilities is installed
    if [[ -f "/data/on_boot.sh" ]]; then
        log_success "unifios-utilities on-boot-script is installed"
    else
        log_warning "unifios-utilities on-boot-script not detected"
        log_info "Will install it if you proceed with installation"
    fi
}

check_network_connectivity() {
    log_info "Checking network connectivity..."
    
    # Check DNS resolution
    if host github.com &>/dev/null; then
        log_success "DNS resolution working"
    else
        log_warning "DNS resolution may have issues"
    fi
    
    # Check GitHub connectivity
    if curl -s -o /dev/null -w "%{http_code}" https://api.github.com | grep -q "200"; then
        log_success "Can reach GitHub API"
    else
        log_warning "Cannot reach GitHub API - installation may fail"
    fi
}

check_aws_tools() {
    log_info "Checking AWS tools..."
    
    # Check if AWS CLI is installed
    if command -v aws &> /dev/null; then
        local aws_version=$(aws --version 2>&1)
        log_info "AWS CLI found: ${aws_version}"
    else
        log_info "AWS CLI not found (optional - dddns uses built-in SDK)"
    fi
    
    # Check for AWS credentials
    if [[ -f "$HOME/.aws/credentials" ]] || [[ -f "/root/.aws/credentials" ]]; then
        log_success "AWS credentials file found"
    elif [[ -n "${AWS_ACCESS_KEY_ID}" ]]; then
        log_success "AWS environment variables detected"
    else
        log_warning "No AWS credentials found - you'll need to configure them"
    fi
}

check_existing_installation() {
    log_info "Checking for existing dddns installation..."
    
    local found_installation=false
    
    # Check binary
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        local version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
        log_info "Found dddns binary: version ${version}"
        found_installation=true
    fi
    
    # Check symlink
    if [[ -L "/usr/local/bin/${BINARY_NAME}" ]]; then
        log_info "Found symlink in /usr/local/bin"
        found_installation=true
    fi
    
    # Check config
    if [[ -f "${CONFIG_DIR}/config.yaml" ]]; then
        log_info "Found configuration file"
        found_installation=true
    fi
    
    # Check cron
    if [[ -f "${CRON_FILE}" ]]; then
        log_info "Found cron job"
        found_installation=true
    fi
    
    # Check boot script
    if [[ -f "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}" ]]; then
        log_info "Found boot script"
        found_installation=true
    fi
    
    if [[ "$found_installation" == "true" ]]; then
        log_warning "Existing installation detected"
        EXISTING_INSTALL=true
    else
        log_success "No existing installation found"
        EXISTING_INSTALL=false
    fi
}

# ============================================================================
# Installation Functions
# ============================================================================

install_on_boot_script() {
    if [[ ! -f "/data/on_boot.sh" ]]; then
        log_info "Installing unifios-utilities on-boot-script..."
        
        if curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash; then
            log_success "on-boot-script installed successfully"
        else
            log_error "Failed to install on-boot-script"
            log_info "Continuing anyway - you may need to install it manually"
        fi
    fi
}

get_latest_version() {
    local version="${1:-latest}"
    
    if [[ "$version" == "latest" ]]; then
        log_info "Fetching latest version from GitHub..."
        version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
                  grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        
        if [[ -z "$version" ]]; then
            log_error "Failed to get latest version from GitHub"
            exit 1
        fi
    fi
    
    echo "$version"
}

download_binary() {
    local version="$1"
    local force="${2:-false}"
    
    # Check if already installed
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]] && [[ "$force" != "true" ]]; then
        local current=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
        if [[ "$current" == "$version" ]]; then
            log_success "dddns ${version} is already installed"
            return 0
        fi
    fi
    
    log_info "Downloading dddns ${version}..."
    
    # Create installation directory
    mkdir -p "${INSTALL_DIR}"
    
    # Download binary
    local url="https://github.com/${GITHUB_REPO}/releases/download/${version}/dddns-linux-${ARCH}"
    log_debug "Download URL: ${url}"
    
    if curl -L -o "${INSTALL_DIR}/${BINARY_NAME}.tmp" "$url"; then
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}.tmp"
        mv "${INSTALL_DIR}/${BINARY_NAME}.tmp" "${INSTALL_DIR}/${BINARY_NAME}"
        log_success "Binary downloaded successfully"
    else
        log_error "Failed to download binary"
        exit 1
    fi
}

create_boot_script() {
    log_info "Creating boot persistence script..."
    
    mkdir -p "${BOOT_SCRIPT_DIR}"
    
    cat > "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}" << 'EOF'
#!/bin/bash
#
# dddns Boot Script
# Ensures dddns is available after reboot
#

BINARY_PATH="/data/dddns/dddns"
SYMLINK_PATH="/usr/local/bin/dddns"
CONFIG_DIR="/data/.dddns"
CRON_FILE="/etc/cron.d/dddns"
LOG_FILE="/var/log/dddns-boot.log"

echo "[$(date)] Starting dddns boot script" >> $LOG_FILE

# Create symlink if it doesn't exist
if [[ -f "$BINARY_PATH" ]] && [[ ! -L "$SYMLINK_PATH" ]]; then
    ln -sf "$BINARY_PATH" "$SYMLINK_PATH"
    echo "[$(date)] Created symlink: $SYMLINK_PATH" >> $LOG_FILE
fi

# Ensure config directory exists with correct permissions
if [[ ! -d "$CONFIG_DIR" ]]; then
    mkdir -p "$CONFIG_DIR"
    chmod 700 "$CONFIG_DIR"
    echo "[$(date)] Created config directory: $CONFIG_DIR" >> $LOG_FILE
fi

# Re-create cron job
cat > "$CRON_FILE" << 'CRON'
# dddns - Dynamic DNS updater for Route53
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Run every 30 minutes
*/30 * * * * root /usr/local/bin/dddns update >> /var/log/dddns.log 2>&1
CRON

# Restart cron to pick up changes
if /etc/init.d/cron restart >/dev/null 2>&1; then
    echo "[$(date)] Cron service restarted" >> $LOG_FILE
else
    echo "[$(date)] Failed to restart cron" >> $LOG_FILE
fi

echo "[$(date)] dddns boot script completed" >> $LOG_FILE
EOF
    
    chmod +x "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    log_success "Boot script created: ${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
}

setup_configuration() {
    if [[ -f "${CONFIG_DIR}/config.yaml" ]]; then
        log_info "Configuration file already exists"
        return 0
    fi
    
    log_info "Creating default configuration..."
    
    mkdir -p "${CONFIG_DIR}"
    chmod 700 "${CONFIG_DIR}"
    
    cat > "${CONFIG_DIR}/config.yaml" << 'EOF'
# dddns Configuration for UDM
# Please update with your AWS and DNS settings

# AWS Settings
aws_profile: ""          # AWS CLI profile name (optional)
aws_region: "us-east-1"  # AWS region

# DNS Settings (required)
hosted_zone_id: ""       # Your Route53 Hosted Zone ID
hostname: ""             # Domain to update (e.g., home.example.com)
ttl: 300                 # TTL in seconds

# Operational Settings
ip_cache_file: "/data/.dddns/last-ip.txt"
skip_proxy_check: false
EOF
    
    chmod 600 "${CONFIG_DIR}/config.yaml"
    log_success "Configuration template created: ${CONFIG_DIR}/config.yaml"
    log_warning "Please edit the configuration with your AWS settings"
}

# ============================================================================
# Uninstall Function
# ============================================================================

uninstall() {
    log_warning "Uninstalling dddns..."
    
    # Stop cron job
    if [[ -f "${CRON_FILE}" ]]; then
        rm -f "${CRON_FILE}"
        log_info "Removed cron job"
    fi
    
    # Remove boot script
    if [[ -f "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}" ]]; then
        rm -f "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
        log_info "Removed boot script"
    fi
    
    # Remove symlink
    if [[ -L "/usr/local/bin/${BINARY_NAME}" ]]; then
        rm -f "/usr/local/bin/${BINARY_NAME}"
        log_info "Removed symlink"
    fi
    
    # Remove binary
    if [[ -d "${INSTALL_DIR}" ]]; then
        rm -rf "${INSTALL_DIR}"
        log_info "Removed binary"
    fi
    
    log_warning "Configuration preserved at ${CONFIG_DIR}"
    log_info "To remove configuration: rm -rf ${CONFIG_DIR}"
    
    log_success "dddns uninstalled"
}

# ============================================================================
# Environment Check Summary
# ============================================================================

print_environment_summary() {
    echo ""
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  Environment Check Summary${NC}"
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    
    echo -e "\n${GREEN}System:${NC}"
    echo "  • Architecture: ARM64 ✓"
    echo "  • Persistent storage: /data ✓"
    echo "  • Root access: Yes ✓"
    
    echo -e "\n${GREEN}Boot Persistence:${NC}"
    if [[ "$ON_BOOT_EXISTS" == "true" ]]; then
        echo "  • on-boot scripts: Configured ✓"
    else
        echo "  • on-boot scripts: Will be installed"
    fi
    
    echo -e "\n${GREEN}Installation Status:${NC}"
    if [[ "$EXISTING_INSTALL" == "true" ]]; then
        echo "  • dddns: Already installed (can upgrade)"
    else
        echo "  • dddns: Not installed (ready to install)"
    fi
    
    echo ""
}

# ============================================================================
# Main Installation Flow
# ============================================================================

main() {
    local action="install"
    local version="latest"
    local force="false"
    local check_only="false"
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --force)
                force="true"
                shift
                ;;
            --version)
                version="$2"
                shift 2
                ;;
            --uninstall)
                action="uninstall"
                shift
                ;;
            --check-only)
                check_only="true"
                shift
                ;;
            --debug)
                DEBUG=1
                shift
                ;;
            --help)
                cat << HELP
Usage: $0 [OPTIONS]

Options:
  --force          Force reinstall even if already installed
  --version VER    Install specific version (default: latest)
  --uninstall      Remove dddns installation
  --check-only     Only run environment checks
  --debug          Enable debug output
  --help           Show this help message

Examples:
  $0                    # Install latest version
  $0 --check-only       # Only check environment
  $0 --force            # Force reinstall
  $0 --version v1.0.0   # Install specific version
  $0 --uninstall        # Remove installation
HELP
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    print_header
    
    # Run environment checks
    log_info "Running environment checks..."
    echo ""
    
    check_root
    detect_architecture
    detect_device_info
    check_persistent_storage
    check_on_boot_script
    check_network_connectivity
    check_aws_tools
    check_existing_installation
    
    print_environment_summary
    
    # Exit if check-only
    if [[ "$check_only" == "true" ]]; then
        log_success "Environment check complete"
        exit 0
    fi
    
    # Handle uninstall
    if [[ "$action" == "uninstall" ]]; then
        if [[ "$EXISTING_INSTALL" != "true" ]]; then
            log_error "No installation found to uninstall"
            exit 1
        fi
        
        read -p "Are you sure you want to uninstall dddns? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            uninstall
        else
            log_info "Uninstall cancelled"
        fi
        exit 0
    fi
    
    # Confirm installation
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    if [[ "$EXISTING_INSTALL" == "true" ]]; then
        echo -e "${YELLOW}Ready to upgrade dddns${NC}"
    else
        echo -e "${GREEN}Ready to install dddns${NC}"
    fi
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    
    read -p "Continue with installation? [Y/n] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]] && [[ -n $REPLY ]]; then
        log_info "Installation cancelled"
        exit 0
    fi
    
    # Perform installation
    echo ""
    log_info "Starting installation..."
    
    # Install on-boot-script if needed
    if [[ "$ON_BOOT_EXISTS" != "true" ]] || [[ ! -f "/data/on_boot.sh" ]]; then
        install_on_boot_script
    fi
    
    # Get version
    version=$(get_latest_version "$version")
    log_info "Installing version: ${version}"
    
    # Download and install binary
    download_binary "$version" "$force"
    
    # Create boot script
    create_boot_script
    
    # Setup configuration
    setup_configuration
    
    # Create symlink
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
    
    # Run boot script to apply changes immediately
    log_info "Applying configuration..."
    "${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
    
    # Test installation
    echo ""
    log_info "Testing installation..."
    if "${INSTALL_DIR}/${BINARY_NAME}" --version &>/dev/null; then
        local installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version)
        log_success "dddns ${installed_version} installed successfully"
    else
        log_error "Installation test failed"
        exit 1
    fi
    
    # Final summary
    echo ""
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}  Installation Complete!${NC}"
    echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "Next steps:"
    echo "1. Edit configuration: vi ${CONFIG_DIR}/config.yaml"
    echo "2. Add your AWS credentials and Route53 settings"
    echo "3. Test: dddns update --dry-run"
    echo "4. Check logs: tail -f ${LOG_FILE}"
    echo ""
    echo "The cron job will run every 30 minutes automatically."
    echo ""
}

# Run main function
main "$@"