#!/bin/bash
#
# Test harness for install.sh functions
# This script allows testing each function independently
#

set -e

# Configuration (same as install.sh)
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
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# Test mode flags
DRY_RUN=false
VERBOSE=true
STEP_MODE=true

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1" >&2; }
log_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
log_debug() { [[ "$VERBOSE" == true ]] && echo -e "${CYAN}[DEBUG]${NC} $1"; }

# Test wrapper function
test_function() {
    local func_name="$1"
    local description="$2"

    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}Testing:${NC} $func_name"
    echo -e "${BLUE}Description:${NC} $description"
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

    if [[ "$STEP_MODE" == true ]]; then
        echo -e "${YELLOW}Press ENTER to run this test, 's' to skip, or 'q' to quit...${NC}"
        read -r response < /dev/tty
        case "$response" in
            s|S)
                log_warning "Skipping $func_name"
                return 0
                ;;
            q|Q)
                log_info "Exiting test harness"
                exit 0
                ;;
        esac
    fi

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would execute: $func_name"
    else
        # Execute the function
        if $func_name; then
            log_success "$func_name completed successfully"
        else
            log_error "$func_name failed with exit code $?"
            if [[ "$STEP_MODE" == true ]]; then
                echo -e "${YELLOW}Continue anyway? (y/N)${NC}"
                read -r response < /dev/tty
                if [[ "$response" != "y" && "$response" != "Y" ]]; then
                    exit 1
                fi
            fi
        fi
    fi
}

# ============================================================================
# Functions from install.sh (to be tested)
# ============================================================================

# Check if running as root
check_root() {
    log_debug "Checking if running as root (EUID: $EUID)"
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        return 1
    fi
    log_success "Running as root"
    return 0
}

# Detect architecture
detect_arch() {
    local arch=$(uname -m)
    log_debug "System architecture: $arch"

    case "$arch" in
        aarch64|arm64)
            ARCH="arm64"
            ;;
        x86_64|amd64)
            ARCH="amd64"
            ;;
        armv7l|armhf)
            ARCH="arm"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            return 1
            ;;
    esac

    log_success "Detected architecture: $ARCH"
    return 0
}

# Check if running on UDM
check_udm() {
    log_debug "Checking for UDM environment"

    if [[ -f /etc/unifi-os/unifi-os.conf ]]; then
        log_success "Running on Ubiquiti Dream Machine"
        if [[ "$VERBOSE" == true ]]; then
            log_debug "UDM configuration:"
            cat /etc/unifi-os/unifi-os.conf | head -5
        fi
        return 0
    else
        log_error "Not running on a Ubiquiti Dream Machine"
        log_info "This script is designed for UDM/UDR devices"
        return 1
    fi
}

# Check for existing installation
check_existing() {
    log_debug "Checking for existing dddns installation"

    if [[ -f "$INSTALL_DIR/$BINARY_NAME" ]]; then
        log_warning "Found existing installation at $INSTALL_DIR/$BINARY_NAME"

        # Try to get version
        if "$INSTALL_DIR/$BINARY_NAME" --version &>/dev/null; then
            local version=$("$INSTALL_DIR/$BINARY_NAME" --version 2>/dev/null | head -1)
            log_info "Current version: $version"
        fi

        EXISTING_INSTALL=true
        return 0
    else
        log_info "No existing installation found"
        EXISTING_INSTALL=false
        return 0
    fi
}

# Create required directories
create_directories() {
    log_debug "Creating required directories"

    # Installation directory
    if [[ ! -d "$INSTALL_DIR" ]]; then
        log_info "Creating $INSTALL_DIR"
        [[ "$DRY_RUN" == false ]] && mkdir -p "$INSTALL_DIR"
    else
        log_debug "$INSTALL_DIR already exists"
    fi

    # Config directory
    if [[ ! -d "$CONFIG_DIR" ]]; then
        log_info "Creating $CONFIG_DIR"
        [[ "$DRY_RUN" == false ]] && mkdir -p "$CONFIG_DIR"
    else
        log_debug "$CONFIG_DIR already exists"
    fi

    # Boot script directory
    if [[ ! -d "$BOOT_SCRIPT_DIR" ]]; then
        log_info "Creating $BOOT_SCRIPT_DIR"
        [[ "$DRY_RUN" == false ]] && mkdir -p "$BOOT_SCRIPT_DIR"
    else
        log_debug "$BOOT_SCRIPT_DIR already exists"
    fi

    log_success "All directories ready"
    return 0
}

# Get latest release info
get_release_info() {
    log_debug "Fetching latest release information"

    local api_url="https://api.github.com/repos/$GITHUB_REPO/releases/latest"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would fetch from: $api_url"
        DOWNLOAD_URL="https://example.com/dddns-linux-arm64"
        VERSION="v0.1.0"
        return 0
    fi

    # Fetch release info
    log_info "Fetching release info from GitHub"
    local release_json=$(curl -sL "$api_url")

    # Extract version
    VERSION=$(echo "$release_json" | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$VERSION" ]]; then
        log_error "Failed to get version from release"
        return 1
    fi

    # Extract download URL for our architecture
    DOWNLOAD_URL=$(echo "$release_json" | grep "browser_download_url.*linux-${ARCH}\"" | cut -d'"' -f4 | head -1)
    if [[ -z "$DOWNLOAD_URL" ]]; then
        log_error "Failed to find download URL for linux-${ARCH}"
        return 1
    fi

    log_success "Found version $VERSION"
    log_info "Download URL: $DOWNLOAD_URL"
    return 0
}

# Download binary
download_binary() {
    log_debug "Downloading dddns binary"

    if [[ -z "$DOWNLOAD_URL" ]]; then
        log_error "No download URL available"
        return 1
    fi

    local temp_file="/tmp/${BINARY_NAME}-${VERSION}"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would download from: $DOWNLOAD_URL"
        log_info "[DRY RUN] Would save to: $temp_file"
        return 0
    fi

    log_info "Downloading $VERSION for linux-${ARCH}"

    if curl -L -o "$temp_file" "$DOWNLOAD_URL" --progress-bar; then
        chmod +x "$temp_file"

        # Verify binary works
        if "$temp_file" --version &>/dev/null; then
            log_success "Binary downloaded and verified"

            # Move to installation directory
            mv "$temp_file" "$INSTALL_DIR/$BINARY_NAME"
            log_success "Binary installed to $INSTALL_DIR/$BINARY_NAME"
        else
            log_error "Downloaded binary failed verification"
            rm -f "$temp_file"
            return 1
        fi
    else
        log_error "Download failed"
        return 1
    fi

    return 0
}

# Create symlink
create_symlink() {
    log_debug "Creating symlink in /usr/local/bin"

    local symlink_path="/usr/local/bin/$BINARY_NAME"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would create symlink: $symlink_path -> $INSTALL_DIR/$BINARY_NAME"
        return 0
    fi

    # Remove existing symlink if present
    if [[ -L "$symlink_path" ]]; then
        log_debug "Removing existing symlink"
        rm -f "$symlink_path"
    fi

    # Create new symlink
    ln -sf "$INSTALL_DIR/$BINARY_NAME" "$symlink_path"

    # Verify symlink
    if [[ -L "$symlink_path" ]] && [[ -f "$symlink_path" ]]; then
        log_success "Symlink created: $symlink_path"
        return 0
    else
        log_error "Failed to create symlink"
        return 1
    fi
}

# Setup boot script
setup_boot_script() {
    log_debug "Setting up boot persistence"

    local boot_script="$BOOT_SCRIPT_DIR/$BOOT_SCRIPT_NAME"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would create boot script: $boot_script"
        return 0
    fi

    cat > "$boot_script" << 'EOF'
#!/bin/bash
# dddns boot script for UDM
# Ensures dddns is available after reboot

# Create symlink
ln -sf /data/dddns/dddns /usr/local/bin/dddns

# Set up cron job
cat > /etc/cron.d/dddns << 'CRON'
# Update DNS every 30 minutes
*/30 * * * * root /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
CRON

# Restart cron to pick up new job
/etc/init.d/cron restart

logger "dddns boot setup completed"
EOF

    chmod +x "$boot_script"
    log_success "Boot script created: $boot_script"

    # Execute boot script to set up cron
    log_info "Executing boot script to set up cron"
    "$boot_script"

    return 0
}

# Verify installation
verify_installation() {
    log_debug "Verifying installation"

    local all_good=true

    # Check binary
    if [[ -f "$INSTALL_DIR/$BINARY_NAME" ]]; then
        log_success "Binary exists: $INSTALL_DIR/$BINARY_NAME"

        if "$BINARY_NAME" --version &>/dev/null; then
            local version=$("$BINARY_NAME" --version 2>/dev/null | head -1)
            log_success "Binary works: $version"
        else
            log_error "Binary doesn't execute properly"
            all_good=false
        fi
    else
        log_error "Binary not found"
        all_good=false
    fi

    # Check symlink
    if [[ -L "/usr/local/bin/$BINARY_NAME" ]]; then
        log_success "Symlink exists: /usr/local/bin/$BINARY_NAME"
    else
        log_error "Symlink not found"
        all_good=false
    fi

    # Check boot script
    if [[ -f "$BOOT_SCRIPT_DIR/$BOOT_SCRIPT_NAME" ]]; then
        log_success "Boot script exists: $BOOT_SCRIPT_DIR/$BOOT_SCRIPT_NAME"
    else
        log_error "Boot script not found"
        all_good=false
    fi

    # Check cron
    if [[ -f "$CRON_FILE" ]]; then
        log_success "Cron job exists: $CRON_FILE"
    else
        log_error "Cron job not found"
        all_good=false
    fi

    # Check config directory
    if [[ -d "$CONFIG_DIR" ]]; then
        log_success "Config directory exists: $CONFIG_DIR"
    else
        log_error "Config directory not found"
        all_good=false
    fi

    if [[ "$all_good" == true ]]; then
        log_success "Installation verified successfully"
        return 0
    else
        log_error "Installation verification failed"
        return 1
    fi
}

# ============================================================================
# Main test execution
# ============================================================================

main() {
    echo ""
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║        dddns Install Script Function Test Harness         ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo ""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run)
                DRY_RUN=true
                log_warning "DRY RUN MODE - No changes will be made"
                ;;
            --no-step)
                STEP_MODE=false
                log_info "Continuous mode - won't pause between steps"
                ;;
            --quiet)
                VERBOSE=false
                ;;
            *)
                log_warning "Unknown option: $1"
                ;;
        esac
        shift
    done

    # Run tests in order
    test_function check_root "Verify script is running as root"
    test_function detect_arch "Detect system architecture"
    test_function check_udm "Verify UDM environment"
    test_function check_existing "Check for existing installation"
    test_function create_directories "Create required directories"
    test_function get_release_info "Fetch latest release information"
    test_function download_binary "Download and install binary"
    test_function create_symlink "Create symlink in PATH"
    test_function setup_boot_script "Set up boot persistence and cron"
    test_function verify_installation "Verify complete installation"

    echo ""
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║                    Test Harness Complete                   ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo ""

    if verify_installation; then
        log_success "All tests passed - installation complete!"
        echo ""
        echo "Next steps:"
        echo "1. Configure dddns: dddns config init"
        echo "2. Test update: dddns update --dry-run"
        echo "3. Run update: dddns update"
    else
        log_warning "Some tests failed - review output above"
    fi
}

# Run main function
main "$@"