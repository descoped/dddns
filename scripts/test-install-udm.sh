#!/bin/bash
#
# Interactive step-by-step UDM installation test script for dddns
# This allows testing each step individually to debug any issues
#

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GITHUB_REPO="descoped/dddns"
INSTALL_DIR="/data/dddns"
CONFIG_DIR="/data/.dddns"
BINARY_NAME="dddns"

# Function to print colored output
print_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[✓]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
print_error() { echo -e "${RED}[✗]${NC} $1"; }

# Function to prompt for continuation
prompt_continue() {
    echo ""
    echo -e "${YELLOW}Press ENTER to continue to next step, or Ctrl+C to abort...${NC}"
    read -r
}

# Function to run a command with description
run_step() {
    local description="$1"
    shift
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}STEP:${NC} $description"
    echo -e "${BLUE}CMD:${NC} $*"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    prompt_continue

    if "$@"; then
        print_success "Step completed successfully"
    else
        print_error "Step failed with exit code $?"
        return 1
    fi
}

# Main installation steps
main() {
    echo ""
    echo "╔════════════════════════════════════════════════════╗"
    echo "║     dddns UDM Installation Test (Step-by-Step)    ║"
    echo "╚════════════════════════════════════════════════════╝"
    echo ""

    # Step 1: Check environment
    print_info "Starting environment checks..."

    run_step "Check if running on UDM" \
        test -f /etc/unifi-os/unifi-os.conf

    run_step "Display system information" \
        cat /etc/unifi-os/unifi-os.conf

    run_step "Check architecture" \
        uname -m

    # Step 2: Check existing installation
    print_info "Checking for existing installation..."

    if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
        print_warning "Found existing installation at $INSTALL_DIR/$BINARY_NAME"
        run_step "Check existing version" \
            "$INSTALL_DIR/$BINARY_NAME" --version || true
    else
        print_info "No existing installation found"
    fi

    # Step 3: Create directories
    print_info "Setting up directories..."

    run_step "Create installation directory" \
        mkdir -p "$INSTALL_DIR"

    run_step "Create config directory" \
        mkdir -p "$CONFIG_DIR"

    run_step "Create on_boot.d directory (if needed)" \
        mkdir -p /data/on_boot.d

    # Step 4: Download binary
    print_info "Downloading dddns binary..."

    # Get latest release URL
    LATEST_RELEASE_URL="https://api.github.com/repos/$GITHUB_REPO/releases/latest"

    run_step "Fetch latest release information" \
        curl -s "$LATEST_RELEASE_URL" -o /tmp/dddns-release.json

    run_step "Parse download URL for linux-arm64" \
        bash -c "grep -o '\"browser_download_url\": \"[^\"]*linux-arm64[^\"]*\"' /tmp/dddns-release.json | cut -d'\"' -f4 | head -1 > /tmp/dddns-url.txt"

    DOWNLOAD_URL=$(cat /tmp/dddns-url.txt)
    if [ -z "$DOWNLOAD_URL" ]; then
        print_error "Failed to find download URL"
        exit 1
    fi

    print_info "Download URL: $DOWNLOAD_URL"

    run_step "Download binary" \
        curl -L -o "$INSTALL_DIR/$BINARY_NAME.tmp" "$DOWNLOAD_URL"

    run_step "Make binary executable" \
        chmod +x "$INSTALL_DIR/$BINARY_NAME.tmp"

    run_step "Move binary to final location" \
        mv "$INSTALL_DIR/$BINARY_NAME.tmp" "$INSTALL_DIR/$BINARY_NAME"

    # Step 5: Create symlink
    print_info "Creating symlink..."

    run_step "Remove old symlink if exists" \
        rm -f /usr/local/bin/$BINARY_NAME || true

    run_step "Create symlink to /usr/local/bin" \
        ln -sf "$INSTALL_DIR/$BINARY_NAME" /usr/local/bin/$BINARY_NAME

    # Step 6: Verify installation
    print_info "Verifying installation..."

    run_step "Check binary version" \
        $BINARY_NAME --version

    run_step "Check binary help" \
        $BINARY_NAME --help

    # Step 7: Create boot script
    print_info "Setting up boot persistence..."

    BOOT_SCRIPT="/data/on_boot.d/20-dddns.sh"

    cat > /tmp/boot-script.sh << 'EOF'
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

echo "dddns boot setup completed"
EOF

    run_step "Create boot script" \
        cp /tmp/boot-script.sh "$BOOT_SCRIPT"

    run_step "Make boot script executable" \
        chmod +x "$BOOT_SCRIPT"

    run_step "Run boot script to set up cron" \
        "$BOOT_SCRIPT"

    # Step 8: Initialize configuration
    print_info "Configuration setup..."

    if [ -f "$CONFIG_DIR/config.yaml" ]; then
        print_warning "Config file already exists at $CONFIG_DIR/config.yaml"
        run_step "Display existing config" \
            cat "$CONFIG_DIR/config.yaml"
    else
        print_info "No config file found"
        echo ""
        echo "You can now run: dddns config init"
        echo "Or create $CONFIG_DIR/config.yaml manually"
    fi

    # Step 9: Test commands
    print_info "Testing dddns commands..."

    run_step "Test IP detection" \
        $BINARY_NAME ip

    run_step "Test config check" \
        $BINARY_NAME config check || true

    # Step 10: Check cron
    print_info "Verifying cron setup..."

    run_step "Check cron.d file" \
        cat /etc/cron.d/dddns

    run_step "Check if cron is running" \
        ps | grep -E "cron|crond" | grep -v grep

    # Summary
    echo ""
    echo "╔════════════════════════════════════════════════════╗"
    echo "║              Installation Test Complete            ║"
    echo "╚════════════════════════════════════════════════════╝"
    echo ""
    print_success "All installation steps have been executed"
    echo ""
    echo "Next steps:"
    echo "1. Configure dddns: dddns config init"
    echo "2. Test update: dddns update --dry-run"
    echo "3. Run actual update: dddns update"
    echo ""
    echo "Files created:"
    echo "  - Binary: $INSTALL_DIR/$BINARY_NAME"
    echo "  - Symlink: /usr/local/bin/$BINARY_NAME"
    echo "  - Boot script: $BOOT_SCRIPT"
    echo "  - Cron job: /etc/cron.d/dddns"
    echo "  - Config dir: $CONFIG_DIR"
}

# Run main function
main "$@"