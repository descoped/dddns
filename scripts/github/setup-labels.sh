#!/bin/bash
# GitHub label setup for descoped/dddns.
#
# Usage: bash scripts/github/setup-labels.sh
#
# Idempotent — safe to re-run; existing labels are updated in place.

set -e

REPO="descoped/dddns"

echo "Setting up labels for $REPO..."
echo ""

# ============================================================
# AREA LABELS
# ============================================================
echo "=== Area Labels ==="

gh label create "core"      --repo $REPO --description "Core update flow, Route53 client, IP detection"        --color "00ADD8" --force 2>/dev/null || true
gh label create "cli"       --repo $REPO --description "CLI commands, flags, user-facing UX"                   --color "1D76DB" --force 2>/dev/null || true
gh label create "server"    --repo $REPO --description "Serve-mode HTTP handler (UniFi bridge)"                --color "0E8A16" --force 2>/dev/null || true
gh label create "security"  --repo $REPO --description "Config encryption, auth, audit log, IAM"               --color "5319E7" --force 2>/dev/null || true
gh label create "platform"  --repo $REPO --description "UDM/Pi/Linux/macOS/Windows specifics, wanip, profiles" --color "C5DEF5" --force 2>/dev/null || true
gh label create "installer" --repo $REPO --description "Install scripts, GoReleaser, justfile"                 --color "FEF2C0" --force 2>/dev/null || true

echo ""

# ============================================================
# TYPE LABELS
# ============================================================
echo "=== Type Labels ==="

gh label create "bug"         --repo $REPO --description "Something isn't working"        --color "d73a4a" --force 2>/dev/null || true
gh label create "enhancement" --repo $REPO --description "New feature or request"         --color "a2eeef" --force 2>/dev/null || true
gh label create "docs"        --repo $REPO --description "Documentation improvements"    --color "0075ca" --force 2>/dev/null || true
gh label create "refactor"    --repo $REPO --description "Code restructuring"             --color "fbca04" --force 2>/dev/null || true
gh label create "test"        --repo $REPO --description "Test improvements"              --color "bfd4f2" --force 2>/dev/null || true

echo ""

# ============================================================
# PRIORITY LABELS
# ============================================================
echo "=== Priority Labels ==="

gh label create "priority: high" --repo $REPO --description "High priority" --color "b60205" --force 2>/dev/null || true
gh label create "priority: low"  --repo $REPO --description "Low priority"  --color "c2e0c6" --force 2>/dev/null || true

echo ""

# ============================================================
# STATUS LABELS
# ============================================================
echo "=== Status Labels ==="

gh label create "blocked"      --repo $REPO --description "Blocked by external dependency"         --color "d93f0b" --force 2>/dev/null || true
gh label create "needs-design" --repo $REPO --description "Needs design work before implementation" --color "e99695" --force 2>/dev/null || true

echo ""
echo "=== Done ==="
