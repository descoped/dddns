#!/bin/bash
#
# Wrapper script to properly run interactive installation tests
# This downloads and executes the test script with proper TTY handling
#

set -e

# Colors
readonly GREEN='\033[0;32m'
readonly BLUE='\033[0;34m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m'

echo -e "${BLUE}[INFO]${NC} Downloading test script..."

# Determine which script to download
SCRIPT_TYPE="${1:-functions}"
case "$SCRIPT_TYPE" in
    functions|func)
        SCRIPT_URL="https://raw.githubusercontent.com/descoped/dddns/main/scripts/test-install-functions.sh"
        SCRIPT_NAME="test-install-functions.sh"
        ;;
    udm)
        SCRIPT_URL="https://raw.githubusercontent.com/descoped/dddns/main/scripts/test-install-udm.sh"
        SCRIPT_NAME="test-install-udm.sh"
        ;;
    install)
        SCRIPT_URL="https://raw.githubusercontent.com/descoped/dddns/main/scripts/install.sh"
        SCRIPT_NAME="install.sh"
        ;;
    *)
        echo -e "${YELLOW}[!]${NC} Unknown script type: $SCRIPT_TYPE"
        echo "Usage: $0 [functions|udm|install] [options]"
        exit 1
        ;;
esac

# Download to temp location
TEMP_SCRIPT="/tmp/${SCRIPT_NAME}"
curl -fsL "$SCRIPT_URL" -o "$TEMP_SCRIPT"

if [[ ! -f "$TEMP_SCRIPT" ]]; then
    echo -e "${YELLOW}[!]${NC} Failed to download script"
    exit 1
fi

chmod +x "$TEMP_SCRIPT"

echo -e "${GREEN}[✓]${NC} Script downloaded to $TEMP_SCRIPT"
echo -e "${BLUE}[INFO]${NC} Running $SCRIPT_NAME..."
echo ""

# Shift to pass remaining arguments
shift

# Execute with proper TTY
exec "$TEMP_SCRIPT" "$@"