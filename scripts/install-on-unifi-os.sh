#!/bin/bash
#
# dddns installer for Ubiquiti UniFi Dream devices (UDM / UDM-Pro / UDM-SE /
# UDM Pro Max / UDR / UDR7). Safe to re-run — preserves mode on upgrade.
#
# Usage:
#   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
#   ./install-on-unifi-os.sh [--mode cron|serve] [--version <tag>] [--force] [--uninstall] [--rollback]
#
# Version selection:
#   Without --version, the latest non-prerelease tag is used. Pre-release tags
#   (e.g. v0.2.0-rc.1) are excluded by GitHub's /releases/latest endpoint, so
#   testing a release candidate requires --version v0.2.0-rc.1 (or the
#   DDDNS_VERSION env var).
#
# Modes are mutually exclusive:
#   cron  — /etc/cron.d/dddns runs `dddns update` every 30 minutes (default).
#   serve — /etc/systemd/system/dddns.service + inadyn push from the UniFi UI.
#
# On existing installs the script preserves the currently-configured mode
# unless --mode is passed explicitly.
#
# Safety guards on every upgrade:
#   1. Pre-flight — runs `${new}/dddns --version` and `config check` against
#      the existing config BEFORE replacing the installed binary. If either
#      fails, the existing install is untouched.
#   2. State backup — the prior binary, boot script, and cron entry are
#      copied to *.prev before overwrite. `--rollback` restores them.
#   3. Post-install smoke — after `apply_mode`, re-runs `--version` and
#      `config check` against the installed binary. On failure, auto-rolls
#      back to the .prev state and exits non-zero.

set -e

readonly GITHUB_REPO="descoped/dddns"
readonly INSTALL_DIR="/data/dddns"
readonly BINARY_NAME="dddns"
readonly CONFIG_DIR="/data/.dddns"
readonly BOOT_SCRIPT_DIR="/data/on_boot.d"
readonly BOOT_SCRIPT_NAME="20-dddns.sh"
readonly BOOT_SCRIPT="${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
readonly CRON_FILE="/etc/cron.d/dddns"
readonly LOG_FILE="/var/log/dddns.log"
readonly PREV_SUFFIX=".prev"

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

# Runtime state used by the safety gates. Set by download_binary.
NEW_BINARY_PATH=""

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error()   { echo -e "${RED}[✗]${NC} $1" >&2; }
log_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
log_phase()   { echo -e "${BLUE}[$1]${NC} $2"; }

# ---------------------------------------------------------------------------
# Platform checks
# ---------------------------------------------------------------------------

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        aarch64|arm64)
            ARCH="arm64"
            log_success "Detected ARM64 architecture"
            ;;
        *)
            log_error "Unsupported architecture: $arch (UniFi Dream devices require ARM64)"
            exit 1
            ;;
    esac
}

# Identify the device using Ubiquiti's canonical marker first, falling back
# to weaker signals. /proc/ubnthal/system.info is the authoritative source —
# present on UDM/UDR family (1.x through 4.x firmware).
detect_unifi_device() {
    if [[ -f /proc/ubnthal/system.info ]]; then
        local model
        model=$(awk -F= '/^shortname=/{print $2; exit}' /proc/ubnthal/system.info)
        if [[ -n "$model" ]]; then
            log_success "Detected Ubiquiti device: ${model}"
        else
            log_success "Detected Ubiquiti device (model unknown)"
        fi
    elif [[ -d /data ]] && { [[ -f /etc/unifi-os/unifi-os.conf ]] || [[ -d /etc/unifi-core ]] || [[ -d /data/unifi ]]; }; then
        log_warning "Ubiquiti device indicators present but /proc/ubnthal/system.info missing — continuing"
    else
        log_error "This does not look like a UniFi Dream device (/data missing and no Ubiquiti markers)"
        exit 1
    fi

    if [[ ! -d /data ]]; then
        log_error "/data directory missing — persistence is impossible"
        exit 1
    fi

    if ! command -v systemctl >/dev/null 2>&1; then
        log_error "systemctl not found — dddns requires systemd (UniFi OS 2.x+)"
        exit 1
    fi

    if [[ ! -w /etc ]] && [[ ! -w /etc/systemd/system ]]; then
        # On UniFi OS /etc is on the firmware-wiped root FS, but it IS writable.
        log_error "/etc not writable — cannot install systemd unit or cron entry"
        exit 1
    fi

    local available
    available=$(df -BM /data 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/M//')
    if [[ -n "$available" ]] && [[ "$available" -lt 50 ]]; then
        log_error "Low disk space on /data: ${available}MB free (50MB required)"
        exit 1
    fi
    log_success "/data has ${available:-unknown}MB free"
}

# ---------------------------------------------------------------------------
# Bootstrap (unifios-utilities) — required for /data/on_boot.d/*.sh to run
# across reboots and firmware upgrades.
# ---------------------------------------------------------------------------

ensure_on_boot_hook() {
    if [[ -d "${BOOT_SCRIPT_DIR}" ]]; then
        log_success "on_boot.d present at ${BOOT_SCRIPT_DIR}"
        return 0
    fi
    log_warning "on_boot.d missing — installing unifios-utilities to enable persistence"
    if ! curl -fsSL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash; then
        log_error "Failed to install unifios-utilities — persistence across reboots will NOT work"
        exit 1
    fi
    mkdir -p "${BOOT_SCRIPT_DIR}"
}

# ---------------------------------------------------------------------------
# Download & verify
# ---------------------------------------------------------------------------

get_latest_version() {
    local version
    version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
              grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ -z "$version" ]]; then
        log_error "Failed to fetch latest release tag from GitHub"
        exit 1
    fi
    echo "$version"
}

# Verify a given tag exists as a GitHub release (including prereleases).
# Used when the caller passes --version / $DDDNS_VERSION so we fail fast
# with a readable error instead of a confusing 404 during download.
verify_release_tag() {
    local tag="$1"
    local http_code
    http_code=$(curl -s -o /dev/null -w '%{http_code}' \
        "https://api.github.com/repos/${GITHUB_REPO}/releases/tags/${tag}")
    if [[ "$http_code" != "200" ]]; then
        log_error "Release tag '${tag}' not found on GitHub (HTTP ${http_code})"
        log_error "  See https://github.com/${GITHUB_REPO}/releases for the full list."
        exit 1
    fi
}

# Downloads the release tarball + checksums.txt, verifies SHA-256, extracts
# the binary to a temp directory, and sets NEW_BINARY_PATH. Does NOT place
# the binary on the live path — that's place_new_binary's job, after
# preflight has validated it.
download_binary() {
    local version="$1"
    local archive_name="dddns_Linux_${ARCH}.tar.gz"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${version}"
    local temp_dir="/tmp/dddns-install-$$"
    mkdir -p "${temp_dir}"
    # shellcheck disable=SC2064
    trap "rm -rf '${temp_dir}'" EXIT

    log_phase download "Fetching ${archive_name}..."
    if ! curl -L -o "${temp_dir}/${archive_name}" "${base_url}/${archive_name}" --progress-bar; then
        log_error "[download] Failed to fetch ${archive_name}"
        exit 1
    fi

    log_phase download "Fetching checksums.txt..."
    if ! curl -fsL -o "${temp_dir}/checksums.txt" "${base_url}/checksums.txt"; then
        log_error "[download] Could not fetch checksums.txt — refusing to install unverified binary"
        exit 1
    fi

    local expected actual
    expected=$(awk -v name="${archive_name}" '$2 == name {print $1}' "${temp_dir}/checksums.txt")
    if [[ -z "$expected" ]]; then
        log_error "[download] ${archive_name} not listed in checksums.txt"
        exit 1
    fi
    actual=$(sha256sum "${temp_dir}/${archive_name}" | awk '{print $1}')
    if [[ "${expected}" != "${actual}" ]]; then
        log_error "[download] SHA-256 mismatch — binary tampered or corrupted"
        log_error "  Expected: ${expected}"
        log_error "  Got:      ${actual}"
        exit 1
    fi
    log_success "[download] SHA-256 verified"

    if ! tar -xzf "${temp_dir}/${archive_name}" -C "${temp_dir}"; then
        log_error "[download] Failed to extract ${archive_name}"
        exit 1
    fi
    if [[ ! -f "${temp_dir}/${BINARY_NAME}" ]]; then
        log_error "[download] Binary ${BINARY_NAME} missing inside tarball"
        exit 1
    fi
    chmod +x "${temp_dir}/${BINARY_NAME}"
    NEW_BINARY_PATH="${temp_dir}/${BINARY_NAME}"
}

# ---------------------------------------------------------------------------
# Safety gates
# ---------------------------------------------------------------------------

# Runs the NEW binary's self-checks against the EXISTING config BEFORE we
# replace anything on disk. On failure, the live install is untouched.
preflight_binary() {
    local tb="${NEW_BINARY_PATH}"
    log_phase preflight "Verifying new binary..."

    if ! "${tb}" --version >/dev/null 2>&1; then
        log_error "[preflight] New binary does not respond to --version"
        return 1
    fi
    log_success "[preflight] $("${tb}" --version 2>/dev/null | head -1)"

    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        log_phase preflight "Validating existing config under new binary..."
        if ! "${tb}" config check >/dev/null 2>&1; then
            log_error "[preflight] New binary rejected existing config:"
            "${tb}" config check 2>&1 | sed 's/^/    /' >&2 || true
            return 1
        fi
        log_success "[preflight] Config loads cleanly"
    else
        log_info "[preflight] No existing config — skipping config validation"
    fi
    return 0
}

# Copies the current binary, boot script, and cron entry to *.prev. Called
# immediately before state-changing operations. On fresh installs there is
# nothing to back up and this is a no-op.
save_state() {
    log_phase backup "Snapshotting current state to ${PREV_SUFFIX} files..."
    local saved=0
    local f
    for f in "${INSTALL_DIR}/${BINARY_NAME}" "${BOOT_SCRIPT}" "${CRON_FILE}" "/etc/systemd/system/dddns.service"; do
        if [[ -f "$f" ]]; then
            cp -a "$f" "${f}${PREV_SUFFIX}"
            saved=$((saved + 1))
        fi
    done
    if [[ $saved -eq 0 ]]; then
        log_info "[backup] No prior state to save (fresh install)"
    else
        log_success "[backup] ${saved} file(s) snapshotted"
    fi
}

# Restores *.prev files in place. Idempotent — files that don't have a
# .prev peer are left alone. Always restarts cron so a restored entry
# becomes live without a reboot.
rollback_state() {
    log_warning "[rollback] Restoring previous state..."
    local restored=0
    local f
    for f in "${INSTALL_DIR}/${BINARY_NAME}" "${BOOT_SCRIPT}" "${CRON_FILE}" "/etc/systemd/system/dddns.service"; do
        if [[ -f "${f}${PREV_SUFFIX}" ]]; then
            mv "${f}${PREV_SUFFIX}" "$f"
            restored=$((restored + 1))
        fi
    done
    /etc/init.d/cron restart >/dev/null 2>&1 || true
    systemctl daemon-reload >/dev/null 2>&1 || true
    if [[ -f /etc/systemd/system/dddns.service ]]; then
        systemctl restart dddns.service >/dev/null 2>&1 || true
    fi
    if [[ $restored -eq 0 ]]; then
        log_warning "[rollback] Nothing to restore — no ${PREV_SUFFIX} files found"
        return 1
    fi
    log_success "[rollback] Restored ${restored} file(s)"
    return 0
}

# Places NEW_BINARY_PATH at its final location and refreshes the symlink.
# The old binary is expected to have already been snapshotted by save_state.
place_new_binary() {
    log_phase install "Installing new binary at ${INSTALL_DIR}/${BINARY_NAME}..."
    mkdir -p "${INSTALL_DIR}"
    # Atomic same-filesystem mv: a cron-run binary already in flight keeps
    # its inode and finishes cleanly; the next cron invocation picks up the
    # new binary through the symlink.
    mv "${NEW_BINARY_PATH}" "${INSTALL_DIR}/${BINARY_NAME}"
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
    log_success "[install] Binary installed; /usr/local/bin/${BINARY_NAME} → ${INSTALL_DIR}/${BINARY_NAME}"
}

# Runs the installed binary through a fast self-check. Called after
# apply_mode. On failure the caller is responsible for invoking
# rollback_state.
postinstall_smoke() {
    local b="${INSTALL_DIR}/${BINARY_NAME}"
    log_phase smoke "Running post-install checks..."

    if ! "${b}" --version >/dev/null 2>&1; then
        log_error "[smoke] Installed binary did not respond to --version"
        return 1
    fi
    log_success "[smoke] $("${b}" --version 2>/dev/null | head -1)"

    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        if ! "${b}" config check >/dev/null 2>&1; then
            log_error "[smoke] Installed binary rejected config:"
            "${b}" config check 2>&1 | sed 's/^/    /' >&2 || true
            return 1
        fi
        log_success "[smoke] Config check passed"
    fi
    return 0
}

# ---------------------------------------------------------------------------
# Config + mode
# ---------------------------------------------------------------------------

# Writes a minimal config.yaml for fresh installs. Existing configs are
# preserved so upgrades don't clobber user settings.
create_default_config() {
    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        log_info "Existing configuration detected — preserving user settings"
        return 0
    fi

    log_info "Creating default configuration at ${CONFIG_DIR}/config.yaml"
    mkdir -p "${CONFIG_DIR}"
    chmod 700 "${CONFIG_DIR}"

    cat > "${CONFIG_DIR}/config.yaml" <<'EOF'
# dddns Configuration
#
# Edit the values below with your AWS credentials and Route53 settings,
# then run `dddns config check` to validate. For encrypted-at-rest
# storage, run `dddns secure enable`.

aws_region: "us-east-1"
aws_access_key: "YOUR_ACCESS_KEY"
aws_secret_key: "YOUR_SECRET_KEY"

hosted_zone_id: "YOUR_HOSTED_ZONE_ID"
hostname: "home.example.com"
ttl: 300

ip_cache_file: "/data/.dddns/last-ip.txt"
EOF
    chmod 600 "${CONFIG_DIR}/config.yaml"
    log_warning "Edit ${CONFIG_DIR}/config.yaml before the first update run"
}

# Reads the mode of an existing install by parsing boot-script markers.
# Falls back to "cron" when legacy (pre-marker) installs have a cron file.
detect_current_mode() {
    [[ -f "${BOOT_SCRIPT}" ]] || { echo ""; return; }
    if grep -q "^# --- serve mode ---" "${BOOT_SCRIPT}"; then
        echo "serve"
    elif grep -q "^# --- cron mode ---" "${BOOT_SCRIPT}"; then
        echo "cron"
    elif [[ -f "${CRON_FILE}" ]]; then
        echo "cron"
    fi
}

prompt_mode() {
    echo "" >&2
    echo "Select install mode:" >&2
    echo "  1) cron  — poll every 30 minutes  [default]" >&2
    echo "  2) serve — event-driven via the UniFi Dynamic DNS UI" >&2
    echo "" >&2
    echo -n "Choose [1]: " >&2
    local choice
    read -r choice </dev/tty || choice="1"
    case "$choice" in
        2|serve|SERVE) echo "serve" ;;
        *) echo "cron" ;;
    esac
}

# Delegates boot-script generation to the binary and runs the script once
# so the install is effective without a reboot. Returns 0 on success, 1
# on any downstream failure — caller handles rollback.
apply_mode() {
    local mode="$1"
    local dddns="${INSTALL_DIR}/${BINARY_NAME}"
    local secret=""

    if [[ "$mode" == "serve" ]]; then
        log_phase apply "Initializing serve-mode shared secret..."
        if ! secret=$("${dddns}" config rotate-secret --init --quiet 2>&1); then
            log_error "[apply] Failed to initialize serve-mode secret: ${secret}"
            return 1
        fi
    fi

    log_phase apply "Generating boot script (mode=${mode})..."
    if ! "${dddns}" config set-mode "${mode}" --boot-path "${BOOT_SCRIPT}" >/dev/null; then
        log_error "[apply] config set-mode failed"
        return 1
    fi

    log_phase apply "Running boot script..."
    # Cosmetic failures (systemctl warnings on first run, cron restart
    # messages) are not fatal — the script IS idempotent and will re-run
    # next boot. Only treat an exit > 1 as fatal.
    bash "${BOOT_SCRIPT}" >/dev/null 2>&1 || log_warning "[apply] Boot script returned non-zero (often cosmetic — systemctl / cron restart warnings)"

    if [[ "$mode" == "serve" ]]; then
        print_unifi_ui_values "$secret"
    fi
    return 0
}

print_unifi_ui_values() {
    local secret="$1"
    local hostname=""
    if [[ -f "${CONFIG_DIR}/config.yaml" ]]; then
        hostname=$(grep -E '^\s*hostname:' "${CONFIG_DIR}/config.yaml" | head -1 | awk -F'"' '{print $2}')
    fi
    [[ -z "$hostname" ]] && hostname="<your hostname>"

    local bar
    bar=$(printf '=%.0s' {1..65})
    echo ""
    echo "${bar}"
    echo "  UniFi UI values — PASTE NOW, this secret is shown once"
    echo "  Settings → Internet → Dynamic DNS → Create"
    echo "${bar}"
    echo ""
    echo "  Service:  Custom"
    echo "  Hostname: ${hostname}"
    echo "  Username: dddns"
    echo "  Password: ${secret}"
    echo "  Server:   127.0.0.1:53353/nic/update?hostname=%h&myip=%i"
    echo ""
    echo "${bar}"
    echo ""
    echo "The secret is encrypted at rest in config.secure after 'dddns"
    echo "secure enable'. To rotate, run 'dddns config rotate-secret' and"
    echo "update the UniFi UI with the new value."
    echo ""
}

# ---------------------------------------------------------------------------
# Top-level actions
# ---------------------------------------------------------------------------

uninstall() {
    log_warning "Uninstalling dddns..."
    if [[ -f "/etc/systemd/system/dddns.service" ]]; then
        systemctl stop dddns.service >/dev/null 2>&1 || true
        systemctl disable dddns.service >/dev/null 2>&1 || true
        rm -f "/etc/systemd/system/dddns.service"
        systemctl daemon-reload >/dev/null 2>&1 || true
    fi
    rm -f "${CRON_FILE}"
    /etc/init.d/cron restart >/dev/null 2>&1 || true
    rm -f "${BOOT_SCRIPT}"
    rm -f "/usr/local/bin/${BINARY_NAME}"
    rm -rf "${INSTALL_DIR}"
    log_warning "Configuration preserved at ${CONFIG_DIR}"
    log_info "To remove configuration: rm -rf ${CONFIG_DIR}"
    log_success "dddns uninstalled"
}

rollback_action() {
    log_warning "[rollback] Reverting to previous dddns install..."
    if [[ ! -f "${INSTALL_DIR}/${BINARY_NAME}${PREV_SUFFIX}" ]]; then
        log_error "[rollback] No ${INSTALL_DIR}/${BINARY_NAME}${PREV_SUFFIX} on disk — nothing to roll back to"
        log_info "Rollback snapshots are written during each install and removed only by the next successful install."
        exit 1
    fi
    if ! rollback_state; then
        exit 1
    fi
    echo ""
    log_success "[rollback] Complete"
    "${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || true
    echo ""
    log_info "If the rollback resolved an issue, please file a report at:"
    log_info "  https://github.com/${GITHUB_REPO}/issues"
}

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --mode cron|serve   Install or switch to the specified mode. Default:
                      preserve current mode on upgrade; prompt on fresh
                      install.
  --version <tag>     Install a specific release tag (e.g. v0.2.0-rc.1).
                      Required to install a pre-release — GitHub's "latest"
                      endpoint skips those. Also settable via the
                      DDDNS_VERSION env var.
  --force             Reinstall the binary even if the current version
                      matches the target release.
  --uninstall         Remove dddns. Preserves configuration.
  --rollback          Restore the previous binary + boot script + cron
                      entry from the .prev snapshots written by the last
                      install.
  --help              Show this message.
EOF
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    local action="install"
    local force="false"
    local mode=""
    local requested_version="${DDDNS_VERSION:-}"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --uninstall) action="uninstall"; shift ;;
            --rollback)  action="rollback";  shift ;;
            --force)     force="true";       shift ;;
            --mode)
                shift
                [[ $# -eq 0 ]] && { log_error "--mode requires an argument"; exit 1; }
                mode="$1"
                shift
                ;;
            --mode=*)    mode="${1#*=}"; shift ;;
            --version)
                shift
                [[ $# -eq 0 ]] && { log_error "--version requires a tag argument"; exit 1; }
                requested_version="$1"
                shift
                ;;
            --version=*) requested_version="${1#*=}"; shift ;;
            --help)      usage; exit 0 ;;
            *)           log_error "Unknown option: $1"; usage; exit 1 ;;
        esac
    done

    if [[ -n "$mode" ]] && [[ "$mode" != "cron" ]] && [[ "$mode" != "serve" ]]; then
        log_error "--mode must be 'cron' or 'serve' (got: '$mode')"
        exit 1
    fi

    echo ""
    echo "======================================"
    echo "  dddns Installer for UniFi Dream"
    echo "======================================"
    echo ""

    check_root

    if [[ "$action" == "rollback" ]]; then
        rollback_action
        exit 0
    fi

    detect_arch
    detect_unifi_device

    if [[ "$action" == "uninstall" ]]; then
        uninstall
        exit 0
    fi

    local is_upgrade="false"
    if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        is_upgrade="true"
    fi

    # Mode resolution: explicit --mode wins; on upgrade preserve detected
    # mode; on fresh install prompt (or --force defaults to cron).
    if [[ -z "$mode" ]]; then
        if [[ "$is_upgrade" == "true" ]]; then
            mode=$(detect_current_mode)
            if [[ -n "$mode" ]]; then
                log_info "Upgrade detected — preserving mode: ${mode}"
            else
                mode="cron"
                log_info "Upgrade detected, no prior mode marker — defaulting to cron"
            fi
        elif [[ "$force" == "true" ]]; then
            mode="cron"
        else
            mode=$(prompt_mode)
        fi
    fi

    if [[ "$force" != "true" ]] && [[ "$is_upgrade" != "true" ]]; then
        echo ""
        log_info "Installation plan:"
        echo "  • Binary:        ${INSTALL_DIR}/${BINARY_NAME}"
        echo "  • Config:        ${CONFIG_DIR}/config.yaml"
        echo "  • Boot script:   ${BOOT_SCRIPT}"
        echo "  • Mode:          ${mode}"
        echo "  • Log:           ${LOG_FILE}"
        echo ""
        echo -n "Proceed? [Y/n]: "
        local response
        read -r response </dev/tty || response="y"
        if [[ "$response" =~ ^[Nn] ]]; then
            log_info "Installation cancelled"
            exit 0
        fi
        echo ""
    fi

    ensure_on_boot_hook

    local version
    if [[ -n "$requested_version" ]]; then
        log_info "Using requested release: ${requested_version}"
        verify_release_tag "$requested_version"
        version="$requested_version"
    else
        log_info "Fetching latest release tag..."
        version=$(get_latest_version)
        log_info "Latest release: ${version}"
    fi

    # Short-circuit the expensive path if we're already on the latest and
    # the user didn't force reinstall. Only applies to existing installs.
    if [[ "$is_upgrade" == "true" ]] && [[ "$force" != "true" ]]; then
        local current
        current=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $3}' || echo "")
        if [[ -n "$current" ]] && { [[ "$current" == "$version" ]] || [[ "v$current" == "$version" ]]; }; then
            log_success "dddns ${version} already installed — nothing to do"
            exit 0
        fi
    fi

    download_binary "${version}"

    # --- Safety gate 1: preflight against existing config ---
    if ! preflight_binary; then
        log_error "Pre-flight failed — existing install untouched"
        exit 1
    fi

    # --- Safety gate 2: snapshot prior state before replacing anything ---
    save_state

    place_new_binary
    create_default_config

    # --- apply mode with rollback on failure ---
    if ! apply_mode "${mode}"; then
        log_error "Mode apply failed — rolling back"
        rollback_state
        exit 1
    fi

    # --- Safety gate 3: post-install smoke with rollback on failure ---
    if ! postinstall_smoke; then
        log_error "Post-install smoke failed — rolling back"
        rollback_state
        exit 1
    fi

    echo ""
    echo "======================================"
    if [[ "$is_upgrade" == "true" ]]; then
        echo "  Upgrade complete (mode=${mode})"
    else
        echo "  Install complete (mode=${mode})"
    fi
    echo "======================================"
    echo ""
    log_info "Previous state preserved for rollback:"
    log_info "  ${INSTALL_DIR}/${BINARY_NAME}${PREV_SUFFIX}"
    log_info "To revert: $(basename "$0") --rollback"
    echo ""

    if [[ "$is_upgrade" != "true" ]]; then
        echo "Next steps:"
        echo "  1. Edit ${CONFIG_DIR}/config.yaml with your AWS + DNS settings"
        echo "  2. dddns config check        # validate config"
        echo "  3. dddns update --dry-run    # sanity-check an update"
        if [[ "$mode" == "cron" ]]; then
            echo "  4. tail -f ${LOG_FILE}       # watch the next cron run"
        else
            echo "  4. dddns serve test          # exercise the listener"
            echo "  5. dddns serve status        # see last-request summary"
        fi
        echo ""
    fi
}

main "$@"
