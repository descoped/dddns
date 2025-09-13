#!/bin/bash
#
# Quick test runner for UDM - downloads and runs test scripts properly
#

echo "Downloading test script..."
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/test-install-functions.sh -o /tmp/test-install.sh
chmod +x /tmp/test-install.sh
echo "Running test script..."
/tmp/test-install.sh "$@"