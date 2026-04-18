#!/bin/bash
#
# dddns installer for Ubiquiti UniFi Dream devices (UDM / UDM-Pro / UDM-SE /
# UDM Pro Max / UDR / UDR7). Safe to re-run — preserves mode on upgrade.
#
# Usage:
#   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
#   ./install-on-unifi-os.sh [--mode cron|serve] [--version <tag>]
#                            [--force] [--verbose]
#                            [--probe | --uninstall | --rollback]
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
#
# --probe prints a privacy-safe health snapshot (no IPs, no config values,
# no log contents) without changing any state. Output is designed to be
# pasted in a GitHub issue.

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

readonly GITHUB_REPO="descoped/dddns"
readonly INSTALL_DIR="/data/dddns"
readonly BINARY_NAME="dddns"
readonly CONFIG_DIR="/data/.dddns"
readonly BOOT_SCRIPT_DIR="/data/on_boot.d"
readonly BOOT_SCRIPT_NAME="20-dddns.sh"
readonly BOOT_SCRIPT="${BOOT_SCRIPT_DIR}/${BOOT_SCRIPT_NAME}"
readonly CRON_FILE="/etc/cron.d/dddns"
readonly LOG_FILE="/var/log/dddns.log"
readonly SYSTEMD_UNIT="/etc/systemd/system/dddns.service"
readonly PREV_SUFFIX=".prev"

# Files snapshotted by save_state and restored by rollback_state. Adding a
# new install artefact? Add it here, not in the two loops.
readonly STATE_FILES=(
    "${INSTALL_DIR}/${BINARY_NAME}"
    "${BOOT_SCRIPT}"
    "${CRON_FILE}"
    "${SYSTEMD_UNIT}"
)

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

# ---------------------------------------------------------------------------
# Runtime state (mutable)
# ---------------------------------------------------------------------------

# Populated by download_binary → preflight_binary → place_new_binary chain.
NEW_BINARY_PATH=""

# Verbose mode. Enabled via --verbose / -v / --debug or DDDNS_DEBUG=1 env var.
VERBOSE="${DDDNS_DEBUG:-0}"

# ENV_STATE is a bash 4+ associative array populated by gather_environment
# and consumed by print_environment / probe_command. The `declare -gA`
# lives inside gather_environment so this script still parses cleanly
# under legacy bash 3.x (macOS default), even though it will only run
# correctly on UniFi OS's bash 5.x.

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

# All log functions write to stderr. Stdout is reserved for function
# return values captured via $(...). Routing log_info/log_success to stdout
# was a latent bug that surfaced once resolve_mode / resolve_version moved
# return values out of globals and into command substitution.
log_info()    { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[✓]${NC} $1" >&2; }
log_error()   { echo -e "${RED}[✗]${NC} $1" >&2; }
log_warning() { echo -e "${YELLOW}[!]${NC} $1" >&2; }
log_phase()   { echo -e "${BLUE}[$1]${NC} $2" >&2; }
log_debug()   { [[ "$VERBOSE" == "1" ]] && echo -e "${YELLOW}[DEBUG]${NC} $1" >&2 || true; }

# vexec runs a command silently unless VERBOSE=1, in which case its stdout
# and stderr stream through. Use for side-effect commands whose output is
# normally uninteresting but critical when debugging a broken install.
vexec() {
    if [[ "$VERBOSE" == "1" ]]; then
        log_debug "exec: $*"
        "$@"
    else
        "$@" >/dev/null 2>&1
    fi
}

# print_banner draws a ==== heading with the given title. Used for the
# installer's start banner and the success block so both stay in sync.
print_banner() {
    local title="$1"
    echo ""
    echo "======================================"
    echo "  ${title}"
    echo "======================================"
    echo ""
}

# prompt_tty reads a line from /dev/tty with a default fallback. If
# /dev/tty isn't readable (non-interactive pipe) the default is returned
# unchanged. Echoes the chosen value on stdout; prompt text goes to stderr.
prompt_tty() {
    local prompt="$1" default="$2" reply
    if [[ ! -r /dev/tty ]]; then
        echo "$default"
        return
    fi
    echo -n "$prompt" >&2
    read -r reply </dev/tty || reply="$default"
    echo "${reply:-$default}"
}

# ---------------------------------------------------------------------------
# Predicates
# ---------------------------------------------------------------------------

# has_config reports whether an existing dddns configuration lives under
# ${CONFIG_DIR}, in either plain or encrypted form.
has_config() {
    [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]
}

# binary_version_line emits the full first line of `--version` output. Safe
# for logging. Silent on failure (returns empty).
binary_version_line() {
    "$1" --version 2>/dev/null | head -1 || true
}

# binary_version_bare emits just the semver token from `--version` output
# (position 3). Used for upgrade short-circuit comparison.
binary_version_bare() {
    "$1" --version 2>/dev/null | awk '{print $3}' || true
}

# elf_info reads the first 20 bytes of a file and reports its ELF class
# (32/64-bit) and machine type (x86-64 / aarch64 / ARM / ...). Used as a
# fallback for `file` on minimal userlands (UniFi OS ships without it).
# Returns empty string + non-zero exit on non-ELF input or read failure.
elf_info() {
    local path="$1"
    local bytes class machine lo hi machine_hex

    bytes=$(od -A n -t x1 -N 20 "$path" 2>/dev/null | tr -d ' \n') || return 1
    [[ "${bytes:0:8}" == "7f454c46" ]] || return 1   # ELF magic

    case "${bytes:8:2}" in
        01) class="32-bit" ;;
        02) class="64-bit" ;;
        *)  class="?"      ;;
    esac

    # e_machine at offset 18, little-endian (low byte first).
    lo="${bytes:36:2}"
    hi="${bytes:38:2}"
    machine_hex="${hi}${lo}"
    case "$machine_hex" in
        003e) machine="x86-64"  ;;
        00b7) machine="aarch64" ;;
        0028) machine="ARM"     ;;
        00f3) machine="RISC-V"  ;;
        *)    machine="e_machine=0x${machine_hex}" ;;
    esac

    printf 'ELF %s %s\n' "$class" "$machine"
}

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

# detect_unifi_device identifies the host as a UniFi Dream device using the
# canonical /proc/ubnthal/system.info marker, falling back to secondary
# signals. Identity only — capability/resource checks are in
# check_prerequisites so the probe can run them independently.
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
}

# check_prerequisites confirms the host can actually host a dddns install:
# /data persistence, systemd, writable /etc, sufficient free space.
check_prerequisites() {
    if [[ ! -d /data ]]; then
        log_error "/data directory missing — persistence is impossible"
        exit 1
    fi

    if ! command -v systemctl >/dev/null 2>&1; then
        log_error "systemctl not found — dddns requires systemd (UniFi OS 2.x+)"
        exit 1
    fi

    if [[ ! -w /etc ]] && [[ ! -w /etc/systemd/system ]]; then
        log_error "/etc not writable — cannot install systemd unit or cron entry"
        exit 1
    fi

    local available
    available=$(df -BM /data 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/M//' || true)
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
    log_warning "  (upstream publishes no checksum; supply-chain trust assumed)"
    if ! curl -fsSL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash; then
        log_error "Failed to install unifios-utilities — persistence across reboots will NOT work"
        exit 1
    fi
    mkdir -p "${BOOT_SCRIPT_DIR}"
}

# ---------------------------------------------------------------------------
# Release discovery
# ---------------------------------------------------------------------------

# get_latest_version queries GitHub for the "latest" release. GitHub excludes
# prereleases from this endpoint; RC tags must use verify_release_tag via
# --version / DDDNS_VERSION.
get_latest_version() {
    local version
    version=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
              grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
    if [[ -z "$version" ]]; then
        log_error "Failed to fetch latest release tag from GitHub"
        exit 1
    fi
    echo "$version"
}

# verify_release_tag checks that a specific tag exists as a GitHub release
# (prerelease or not). Fails fast with a readable error instead of waiting
# for the subsequent download to 404.
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
    log_debug "tag '${tag}' verified on GitHub (HTTP 200)"
}

# ---------------------------------------------------------------------------
# Download & verify
# ---------------------------------------------------------------------------

# fetch_release_artifacts downloads the tarball and checksums.txt into
# ${temp_dir}. The -f flag ensures a 404 surfaces as a download error
# rather than an HTML page that fails SHA-256 later with a confusing
# "binary tampered" message.
fetch_release_artifacts() {
    local temp_dir="$1" archive_name="$2" base_url="$3"
    local progress

    if [[ -t 1 ]]; then progress="--progress-bar"
    else progress="--silent --show-error"
    fi

    log_phase download "Fetching ${archive_name}..."
    # shellcheck disable=SC2086
    if ! curl -fL -o "${temp_dir}/${archive_name}" "${base_url}/${archive_name}" ${progress}; then
        log_error "[download] Failed to fetch ${archive_name} from ${base_url}"
        return 1
    fi

    log_phase download "Fetching checksums.txt..."
    if ! curl -fsL -o "${temp_dir}/checksums.txt" "${base_url}/checksums.txt"; then
        log_error "[download] Could not fetch checksums.txt — refusing to install unverified binary"
        return 1
    fi
    return 0
}

# verify_sha256 compares the SHA-256 of ${file} against the entry in
# ${checksums} matching ${name}. Returns 0 on match, 1 on any mismatch or
# missing entry. The checksums file is the trust anchor for the install.
verify_sha256() {
    local file="$1" name="$2" checksums="$3"
    local expected actual
    expected=$(awk -v n="$name" '$2 == n {print $1}' "$checksums")
    if [[ -z "$expected" ]]; then
        log_error "[download] ${name} not listed in checksums.txt"
        return 1
    fi
    actual=$(sha256sum "$file" | awk '{print $1}')
    if [[ "$expected" != "$actual" ]]; then
        log_error "[download] SHA-256 mismatch — binary tampered or corrupted"
        log_error "  Expected: ${expected}"
        log_error "  Got:      ${actual}"
        return 1
    fi
    log_success "[download] SHA-256 verified"
    return 0
}

# extract_binary untars the release archive and verifies the binary is
# present + executable inside. Sets NEW_BINARY_PATH on success.
extract_binary() {
    local temp_dir="$1" archive_name="$2"
    if ! tar -xzf "${temp_dir}/${archive_name}" -C "${temp_dir}"; then
        log_error "[download] Failed to extract ${archive_name}"
        return 1
    fi
    if [[ ! -f "${temp_dir}/${BINARY_NAME}" ]]; then
        log_error "[download] Binary ${BINARY_NAME} missing inside tarball"
        return 1
    fi
    chmod +x "${temp_dir}/${BINARY_NAME}"
    NEW_BINARY_PATH="${temp_dir}/${BINARY_NAME}"
    log_debug "new binary staged at ${NEW_BINARY_PATH}"
    return 0
}

# download_binary composes the download pipeline. Keeps temp_dir lifecycle
# with an EXIT trap so the binary's parent dir survives past the function
# (needed because place_new_binary runs later and mv's out of it).
download_binary() {
    local version="$1"
    local archive_name="dddns_Linux_${ARCH}.tar.gz"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${version}"
    local temp_dir="/tmp/dddns-install-$$"

    mkdir -p "${temp_dir}"
    # shellcheck disable=SC2064
    trap "rm -rf '${temp_dir}'" EXIT

    fetch_release_artifacts "$temp_dir" "$archive_name" "$base_url" || exit 1
    verify_sha256 "${temp_dir}/${archive_name}" "$archive_name" "${temp_dir}/checksums.txt" || exit 1
    extract_binary "$temp_dir" "$archive_name" || exit 1
}

# ---------------------------------------------------------------------------
# Safety gates
# ---------------------------------------------------------------------------

# validate_config runs `${binary} config check` against the active config
# (yaml or secure) and prints any failure output with consistent indent.
# Phase is a short label ("preflight" / "smoke") used in log lines.
validate_config() {
    local binary="$1" phase="$2"
    local check_out
    if ! check_out=$("$binary" config check 2>&1); then
        log_error "[${phase}] ${binary} rejected existing config:"
        printf '%s\n' "$check_out" | sed 's/^/    /' >&2
        return 1
    fi
    return 0
}

# preflight_binary validates the NEW (downloaded) binary against the
# EXISTING config before anything on disk changes. On failure the live
# install is untouched; the caller aborts.
preflight_binary() {
    local tb="${NEW_BINARY_PATH}"
    log_phase preflight "Verifying new binary..."

    if ! "${tb}" --version >/dev/null 2>&1; then
        log_error "[preflight] New binary does not respond to --version"
        return 1
    fi
    log_success "[preflight] $(binary_version_line "$tb")"

    if has_config; then
        log_phase preflight "Validating existing config under new binary..."
        validate_config "$tb" "preflight" || return 1
        log_success "[preflight] Config loads cleanly"
    else
        log_info "[preflight] No existing config — skipping config validation"
    fi
    return 0
}

# save_state copies every live install artefact to its .prev peer. Called
# once per install, immediately before place_new_binary. Fresh installs
# are a no-op. Uses STATE_FILES as the single source of truth.
save_state() {
    log_phase backup "Snapshotting current state to ${PREV_SUFFIX} files..."
    local saved=0 f
    for f in "${STATE_FILES[@]}"; do
        if [[ -f "$f" ]]; then
            cp -a "$f" "${f}${PREV_SUFFIX}"
            saved=$((saved + 1))
            log_debug "snapshot: $f → ${f}${PREV_SUFFIX}"
        fi
    done
    if [[ $saved -eq 0 ]]; then
        log_info "[backup] No prior state to save (fresh install)"
    else
        log_success "[backup] ${saved} file(s) snapshotted"
    fi
}

# rollback_state mv's every .prev back to its live path. Idempotent: no
# .prev is left alone. Always kicks the cron and systemd supervisors so
# a restored entry becomes live without a reboot.
rollback_state() {
    log_warning "[rollback] Restoring previous state..."
    local restored=0 f
    for f in "${STATE_FILES[@]}"; do
        if [[ -f "${f}${PREV_SUFFIX}" ]]; then
            mv "${f}${PREV_SUFFIX}" "$f"
            restored=$((restored + 1))
            log_debug "restore: ${f}${PREV_SUFFIX} → $f"
        fi
    done
    vexec /etc/init.d/cron restart || true
    vexec systemctl daemon-reload || true
    if [[ -f "${SYSTEMD_UNIT}" ]]; then
        vexec systemctl restart dddns.service || true
    fi
    if [[ $restored -eq 0 ]]; then
        log_warning "[rollback] Nothing to restore — no ${PREV_SUFFIX} files found"
        return 1
    fi
    log_success "[rollback] Restored ${restored} file(s)"
    return 0
}

# place_new_binary mv's the staged binary into the live path and refreshes
# the /usr/local/bin symlink. The mv is atomic on the same filesystem so a
# cron-run binary already in flight keeps its inode and finishes cleanly;
# the next invocation picks up the new binary through the symlink.
place_new_binary() {
    log_phase install "Installing new binary at ${INSTALL_DIR}/${BINARY_NAME}..."
    mkdir -p "${INSTALL_DIR}"
    mv "${NEW_BINARY_PATH}" "${INSTALL_DIR}/${BINARY_NAME}"
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
    log_success "[install] Binary installed; /usr/local/bin/${BINARY_NAME} → ${INSTALL_DIR}/${BINARY_NAME}"
}

# postinstall_smoke runs the now-live binary through --version + config
# check. Called after apply_mode. On failure the caller is responsible for
# invoking rollback_state.
postinstall_smoke() {
    local b="${INSTALL_DIR}/${BINARY_NAME}"
    log_phase smoke "Running post-install checks..."

    if ! "${b}" --version >/dev/null 2>&1; then
        log_error "[smoke] Installed binary did not respond to --version"
        return 1
    fi
    log_success "[smoke] $(binary_version_line "$b")"

    if has_config; then
        validate_config "$b" "smoke" || return 1
        log_success "[smoke] Config check passed"
    fi
    return 0
}

# ---------------------------------------------------------------------------
# Config + mode
# ---------------------------------------------------------------------------

# create_default_config writes a minimal config.yaml for fresh installs.
# Existing configs (yaml or secure) are preserved so upgrades don't
# clobber user settings.
create_default_config() {
    if has_config; then
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

# detect_current_mode reads the mode of an existing install from the boot
# script's marker comment. Falls back to "cron" when legacy (pre-marker)
# installs have a cron file.
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
    local choice
    choice=$(prompt_tty "Choose [1]: " "1")
    case "$choice" in
        2|serve|SERVE) echo "serve" ;;
        *) echo "cron" ;;
    esac
}

# init_serve_secret initializes the serve-mode shared secret via the
# binary's own config subcommand. Captures stderr to a side file so any
# WARN doesn't contaminate the password printed in the UniFi UI block.
# Echoes the secret on stdout; returns non-zero on failure.
init_serve_secret() {
    local dddns="$1"
    log_phase apply "Initializing serve-mode shared secret..."

    local secret_err secret
    secret_err=$(mktemp)
    # shellcheck disable=SC2064
    trap "rm -f '${secret_err}'" RETURN

    if ! secret=$("${dddns}" config rotate-secret --init --quiet 2>"${secret_err}"); then
        log_error "[apply] Failed to initialize serve-mode secret:"
        sed 's/^/    /' "${secret_err}" >&2 || true
        return 1
    fi
    echo "$secret"
}

# apply_mode delegates boot-script generation to the binary and runs the
# script once so the install is effective without a reboot. Returns 0 on
# success, 1 on any downstream failure — caller handles rollback.
apply_mode() {
    local mode="$1"
    local dddns="${INSTALL_DIR}/${BINARY_NAME}"
    local secret=""

    if [[ "$mode" == "serve" ]]; then
        secret=$(init_serve_secret "$dddns") || return 1
    fi

    log_phase apply "Generating boot script (mode=${mode})..."
    if ! vexec "${dddns}" config set-mode "${mode}" --boot-path "${BOOT_SCRIPT}"; then
        log_error "[apply] config set-mode failed"
        return 1
    fi

    log_phase apply "Running boot script..."
    # Cosmetic failures (systemctl warnings on first run, cron restart
    # messages) are not fatal — the script IS idempotent and will re-run
    # next boot. Re-run visibly under --verbose to diagnose breakage.
    vexec bash "${BOOT_SCRIPT}" || log_warning "[apply] Boot script returned non-zero (often cosmetic — systemctl / cron restart warnings; re-run with --verbose to see output)"

    if [[ "$mode" == "serve" ]]; then
        print_unifi_ui_values "$secret"
    fi
    return 0
}

# print_unifi_ui_values emits a copy-pasteable block for the UniFi
# Dynamic DNS UI. The secret is not persisted anywhere else in plain text.
print_unifi_ui_values() {
    local secret="$1"
    local hostname=""
    if [[ -f "${CONFIG_DIR}/config.yaml" ]]; then
        hostname=$(grep -E '^\s*hostname:' "${CONFIG_DIR}/config.yaml" | head -1 | awk -F'"' '{print $2}' || true)
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
# Environment / state gathering (privacy-safe)
# ---------------------------------------------------------------------------

# gather_environment populates the global ENV_STATE associative array with
# metadata about the current host and install. No IPs, no config values, no
# log contents, no user-authored script bodies — pass-through to probe
# output must remain safe to publish in a GitHub issue.
gather_environment() {
    local target_version="$1" target_mode="$2"

    declare -gA ENV_STATE
    ENV_STATE[device_model]="unknown"
    ENV_STATE[arch_host]="$(uname -m)"
    ENV_STATE[arch_target]="${ARCH:-unknown}"
    ENV_STATE[mode_target]="${target_mode:-(not resolved)}"
    ENV_STATE[version_target]="${target_version:-<latest>}"

    if [[ -f /proc/ubnthal/system.info ]]; then
        local model
        model=$(awk -F= '/^shortname=/{print $2; exit}' /proc/ubnthal/system.info || true)
        [[ -n "$model" ]] && ENV_STATE[device_model]="$model"
    fi
    if [[ -r /proc/version ]]; then
        ENV_STATE[kernel]=$(awk '{print $3}' /proc/version || echo unknown)
    else
        ENV_STATE[kernel]="unknown"
    fi
    ENV_STATE[systemd_version]=$(systemctl --version 2>/dev/null | awk 'NR==1{print $2}' || echo unknown)

    ENV_STATE[install_dir]="${INSTALL_DIR}"
    ENV_STATE[config]="(none)"
    if [[ -f "${CONFIG_DIR}/config.secure" ]]; then
        ENV_STATE[config]="${CONFIG_DIR}/config.secure (encrypted, $(stat -c%s "${CONFIG_DIR}/config.secure") bytes)"
    elif [[ -f "${CONFIG_DIR}/config.yaml" ]]; then
        ENV_STATE[config]="${CONFIG_DIR}/config.yaml ($(stat -c%s "${CONFIG_DIR}/config.yaml") bytes)"
    fi
    ENV_STATE[bootscript]="(none)"
    if [[ -f "${BOOT_SCRIPT}" ]]; then
        ENV_STATE[bootscript]="${BOOT_SCRIPT} ($(stat -c%s "${BOOT_SCRIPT}") bytes)"
    fi
    ENV_STATE[log_file]="(none)"
    if [[ -f "${LOG_FILE}" ]]; then
        local size mtime
        size=$(du -h "${LOG_FILE}" 2>/dev/null | awk '{print $1}' || echo "?")
        mtime=$(stat -c%y "${LOG_FILE}" 2>/dev/null | cut -d. -f1 || echo "?")
        ENV_STATE[log_file]="${LOG_FILE} (${size}, last-modified ${mtime})"
    fi
    ENV_STATE[free_data]="$(df -BM /data 2>/dev/null | awk 'NR==2 {print $4}' || echo unknown)"
    ENV_STATE[verbose]=$([[ "$VERBOSE" == "1" ]] && echo yes || echo no)
}

# print_environment renders the ENV_STATE associative array as a short
# human-readable block. Called after gather_environment.
print_environment() {
    echo ""
    echo "Environment:"
    echo "  • Device:       ${ENV_STATE[device_model]} (${ENV_STATE[arch_host]})"
    echo "  • Arch target:  ${ENV_STATE[arch_target]}"
    echo "  • Mode target:  ${ENV_STATE[mode_target]}"
    echo "  • Version:      ${ENV_STATE[version_target]}"
    echo "  • Install dir:  ${ENV_STATE[install_dir]}"
    echo "  • Config:       ${ENV_STATE[config]}"
    echo "  • Bootscript:   ${ENV_STATE[bootscript]}"
    echo "  • Log file:     ${ENV_STATE[log_file]}"
    echo "  • Free on /data: ${ENV_STATE[free_data]}"
    echo "  • Verbose:      ${ENV_STATE[verbose]}"
    echo ""
}

# ---------------------------------------------------------------------------
# Probe (privacy-safe self-diagnosis)
# ---------------------------------------------------------------------------
#
# Rules baked into the probe code below:
#
#   • Never `cat` a user file. Only `ls`, `stat`, `wc -l`, `du`, `file`.
#   • Never print any IPv4 from `ip addr show`. Count interfaces that pass
#     isPublicIPv4, do not name them.
#   • For the cron entry, print the *schedule field* (first 5 tokens)
#     and the fact that a command follows — not the command body.
#   • Config files: presence + size + mtime only. Never grep, never cat.
#   • Log file: size + mtime only. Never tail.
#
# Output is designed to be copy-pasted into a GitHub issue.

probe_section_header() {
    printf '\n[%s]\n' "$1"
}

probe_section_system() {
    probe_section_header "system"
    printf '  device-model:    %s\n' "${ENV_STATE[device_model]}"
    printf '  arch (host):     %s\n' "${ENV_STATE[arch_host]}"
    printf '  arch (target):   %s\n' "${ENV_STATE[arch_target]}"
    printf '  kernel:          %s\n' "${ENV_STATE[kernel]}"
    printf '  systemd:         %s\n' "${ENV_STATE[systemd_version]}"
    if [[ -f /etc/os-release ]]; then
        local osname
        osname=$(awk -F= '/^PRETTY_NAME=/{gsub(/"/,"",$2); print $2; exit}' /etc/os-release || true)
        printf '  os:              %s\n' "${osname:-unknown}"
    fi
}

probe_section_disk() {
    probe_section_header "disk"
    printf '  /data free:      %s\n' "${ENV_STATE[free_data]}"
    local etcw="no"
    { [[ -w /etc ]] || [[ -w /etc/systemd/system ]]; } && etcw="yes"
    printf '  /etc writable:   %s\n' "$etcw"
}

# probe_cron_schedule extracts the first 5 whitespace-separated tokens
# from the first non-comment non-env line of the cron file. That's the
# crontab schedule — safe. The command body that follows is not printed.
probe_cron_schedule() {
    awk '
        /^[[:space:]]*#/ {next}
        /^[A-Z_]+=/ {next}
        NF >= 6 {
            print $1, $2, $3, $4, $5
            exit
        }
    ' "$1" 2>/dev/null || true
}

probe_section_scheduler() {
    probe_section_header "scheduler"

    local cron_status="unknown"
    if [[ -x /etc/init.d/cron ]]; then
        if /etc/init.d/cron status >/dev/null 2>&1; then cron_status="running"
        else cron_status="not running"
        fi
    elif systemctl is-active cron.service >/dev/null 2>&1; then
        cron_status="running (systemd)"
    fi
    printf '  cron service:    %s\n' "$cron_status"

    if [[ -f "${CRON_FILE}" ]]; then
        local size sched
        size=$(stat -c%s "${CRON_FILE}")
        sched=$(probe_cron_schedule "${CRON_FILE}")
        printf '  cron entry:      present (%s bytes)\n' "$size"
        printf '  cron schedule:   %s\n' "${sched:-(unparseable)}"
    else
        printf '  cron entry:      (absent)\n'
    fi

    if [[ -d "${BOOT_SCRIPT_DIR}" ]]; then
        local count
        count=$(find "${BOOT_SCRIPT_DIR}" -maxdepth 1 -type f | wc -l)
        printf '  on_boot.d:       %s script(s)\n' "$count"
        local f
        while IFS= read -r f; do
            [[ -z "$f" ]] && continue
            local name size mtime
            name=$(basename "$f")
            size=$(stat -c%s "$f")
            mtime=$(stat -c%y "$f" 2>/dev/null | cut -d' ' -f1 || echo "?")
            printf '    - %s (%s bytes, %s)\n' "$name" "$size" "$mtime"
        done < <(find "${BOOT_SCRIPT_DIR}" -maxdepth 1 -type f | sort)
    else
        printf '  on_boot.d:       (absent)\n'
    fi
}

probe_section_dddns_install() {
    probe_section_header "dddns install"
    if [[ -L "/usr/local/bin/${BINARY_NAME}" ]]; then
        local target
        target=$(readlink "/usr/local/bin/${BINARY_NAME}" || true)
        printf '  symlink:         /usr/local/bin/%s → %s\n' "${BINARY_NAME}" "${target:-?}"
    elif [[ -f "/usr/local/bin/${BINARY_NAME}" ]]; then
        printf '  symlink:         (not a symlink; real file at /usr/local/bin/%s)\n' "${BINARY_NAME}"
    else
        printf '  symlink:         (absent)\n'
    fi

    if [[ -x "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        local ver arch_detail size
        ver=$(binary_version_line "${INSTALL_DIR}/${BINARY_NAME}")
        size=$(stat -c%s "${INSTALL_DIR}/${BINARY_NAME}")
        # Prefer `file -b` when available; fall back to a bash-native ELF
        # header inspector for minimal userlands like UniFi OS that ship
        # without the `file` command.
        if command -v file >/dev/null 2>&1; then
            arch_detail=$(file -b "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null | awk -F, '{print $1", "$2}')
            [[ -z "$arch_detail" ]] && arch_detail="(unknown)"
        else
            arch_detail=$(elf_info "${INSTALL_DIR}/${BINARY_NAME}" || true)
            [[ -z "$arch_detail" ]] && arch_detail="(not an ELF file)"
        fi
        printf '  binary:          %s (%s bytes)\n' "${INSTALL_DIR}/${BINARY_NAME}" "$size"
        printf '  file type:       %s\n' "$arch_detail"
        printf '  version:         %s\n' "${ver:-?}"
    else
        printf '  binary:          (not installed)\n'
    fi

    printf '  config:          %s\n' "${ENV_STATE[config]}"

    # Cache presence + mtime only. Never print the value.
    local cache_file="${CONFIG_DIR}/last-ip.txt"
    if [[ -f "${cache_file}" ]]; then
        local mtime
        mtime=$(stat -c%y "${cache_file}" 2>/dev/null | cut -d. -f1 || echo "?")
        printf '  IP cache:        present (last-modified %s; value NOT read)\n' "$mtime"
    else
        printf '  IP cache:        (absent)\n'
    fi

    printf '  log file:        %s\n' "${ENV_STATE[log_file]}"

    # Snapshot presence (rollback readiness)
    local prev_binary="${INSTALL_DIR}/${BINARY_NAME}${PREV_SUFFIX}"
    if [[ -f "${prev_binary}" ]]; then
        local prev_ver prev_mtime
        prev_ver=$(binary_version_line "${prev_binary}")
        prev_mtime=$(stat -c%y "${prev_binary}" 2>/dev/null | cut -d. -f1 || echo "?")
        printf '  rollback ready:  yes (%s; snapshotted %s)\n' "${prev_ver:-?}" "$prev_mtime"
    else
        printf '  rollback ready:  no (.prev snapshot absent)\n'
    fi
}

probe_section_systemd() {
    probe_section_header "systemd units"
    if [[ -f "${SYSTEMD_UNIT}" ]]; then
        # `systemctl is-active` always writes a status word to stdout
        # ("active" / "inactive" / "failed" / "activating" ...) AND exits
        # non-zero for anything but "active". Trailing `|| true` discards
        # the exit code without polluting the captured value.
        local active enabled
        active=$(systemctl is-active dddns.service 2>/dev/null || true)
        enabled=$(systemctl is-enabled dddns.service 2>/dev/null || true)
        printf '  dddns.service:   %s (active=%s, enabled=%s)\n' \
            "${SYSTEMD_UNIT}" "${active:-unknown}" "${enabled:-unknown}"
    else
        printf '  dddns.service:   (absent)\n'
    fi
    if [[ -f /etc/systemd/system/udm-boot.service ]]; then
        local active
        active=$(systemctl is-active udm-boot.service 2>/dev/null || true)
        printf '  udm-boot:        present (active=%s)\n' "${active:-unknown}"
    fi
}

# probe_section_network counts interfaces with a publicly-routable IPv4
# without revealing the addresses or interface names. Also reports
# whether the main routing table has a default route (UDR7's policy-based
# routing answers "no" here — that's the condition that makes the wanip
# fallback necessary).
probe_section_network() {
    probe_section_header "network (metadata only)"
    local default_iface="(none — main table has no default)"
    if [[ -r /proc/net/route ]]; then
        local name
        name=$(awk '$2=="00000000" && NR>1 {print $1; exit}' /proc/net/route || true)
        [[ -n "$name" ]] && default_iface="$name"
    fi
    printf '  default route:   %s\n' "$default_iface"

    # Count public IPv4 addresses across interfaces without naming them.
    # Filter rules match wanip's isPublicIPv4: reject loopback,
    # link-local, RFC1918, CGNAT.
    local public_count
    public_count=$(ip -4 -o addr show 2>/dev/null | awk '
        {
            split($4, a, "/")
            ip=a[1]
            if (ip ~ /^127\./) next
            if (ip ~ /^10\./) next
            if (ip ~ /^192\.168\./) next
            if (ip ~ /^172\.(1[6-9]|2[0-9]|3[01])\./) next
            if (ip ~ /^100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\./) next
            if (ip ~ /^169\.254\./) next
            if (ip ~ /^0\./) next
            count++
        }
        END { print count+0 }
    ' || echo 0)
    printf '  public IPv4:     %s interface(s)\n' "$public_count"
}

probe_command() {
    # Fill ENV_STATE with best-effort values (target version/mode unknown
    # in pure probe mode — we're not installing).
    ARCH="${ARCH:-unknown}"
    gather_environment "(n/a)" "(n/a)"

    echo "=== dddns probe ==="
    printf 'generated: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'probe rev: %s\n' "install-on-unifi-os.sh (HEAD)"

    probe_section_system
    probe_section_disk
    probe_section_scheduler
    probe_section_dddns_install
    probe_section_systemd
    probe_section_network

    echo ""
    echo "[privacy]"
    echo "  This probe output contains no WAN IPs, no config values, no log"
    echo "  contents, and no hostnames from user config. Safe to paste in an"
    echo "  issue at https://github.com/${GITHUB_REPO}/issues"
    echo ""
}

# ---------------------------------------------------------------------------
# Top-level actions
# ---------------------------------------------------------------------------

uninstall() {
    log_warning "Uninstalling dddns..."
    if [[ -f "${SYSTEMD_UNIT}" ]]; then
        vexec systemctl stop dddns.service || true
        vexec systemctl disable dddns.service || true
        rm -f "${SYSTEMD_UNIT}"
        vexec systemctl daemon-reload || true
    fi
    rm -f "${CRON_FILE}"
    vexec /etc/init.d/cron restart || true
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
    binary_version_line "${INSTALL_DIR}/${BINARY_NAME}" | sed 's/^/  now running: /'
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
  --verbose, -v       Show all subprocess output (systemctl, cron restart,
                      boot script). Essential when a test build
                      misbehaves. Also enabled by DDDNS_DEBUG=1.
  --probe             Print a privacy-safe self-diagnosis block (no IPs,
                      no config values, no log contents) without changing
                      any state. Output is safe to paste in a GitHub issue.
  --uninstall         Remove dddns. Preserves configuration.
  --rollback          Restore the previous binary + boot script + cron
                      entry from the .prev snapshots written by the last
                      install.
  --help              Show this message.
EOF
}

# ---------------------------------------------------------------------------
# Orchestration
# ---------------------------------------------------------------------------

# Populated by parse_args so run_install / probe dispatch can read them.
CLI_ACTION="install"
CLI_FORCE="false"
CLI_MODE=""
CLI_VERSION="${DDDNS_VERSION:-}"

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --uninstall) CLI_ACTION="uninstall"; shift ;;
            --rollback)  CLI_ACTION="rollback";  shift ;;
            --probe)     CLI_ACTION="probe";     shift ;;
            --force)     CLI_FORCE="true";       shift ;;
            --mode)
                shift
                [[ $# -eq 0 ]] && { log_error "--mode requires an argument"; exit 1; }
                CLI_MODE="$1"; shift ;;
            --mode=*)    CLI_MODE="${1#*=}"; shift ;;
            --version)
                shift
                [[ $# -eq 0 ]] && { log_error "--version requires a tag argument"; exit 1; }
                CLI_VERSION="$1"; shift ;;
            --version=*) CLI_VERSION="${1#*=}"; shift ;;
            --verbose|-v|--debug) VERBOSE=1; shift ;;
            --help|-h)   usage; exit 0 ;;
            *)           log_error "Unknown option: $1"; usage; exit 1 ;;
        esac
    done

    if [[ -n "$CLI_MODE" ]] && [[ "$CLI_MODE" != "cron" ]] && [[ "$CLI_MODE" != "serve" ]]; then
        log_error "--mode must be 'cron' or 'serve' (got: '$CLI_MODE')"
        exit 1
    fi
}

# resolve_mode decides cron vs serve: explicit --mode wins; on upgrade
# preserve detected mode; on fresh install prompt (or --force defaults to
# cron). Echoes the resolved mode.
resolve_mode() {
    local is_upgrade="$1"
    if [[ -n "$CLI_MODE" ]]; then
        log_debug "mode=${CLI_MODE} (explicit --mode)"
        echo "$CLI_MODE"
        return
    fi
    if [[ "$is_upgrade" == "true" ]]; then
        local detected
        detected=$(detect_current_mode)
        if [[ -n "$detected" ]]; then
            log_info "Upgrade detected — preserving mode: ${detected}"
            log_debug "mode=${detected} (detected from existing install)"
            echo "$detected"
            return
        fi
        log_info "Upgrade detected, no prior mode marker — defaulting to cron"
        log_debug "mode=cron (upgrade fallback)"
        echo "cron"
        return
    fi
    if [[ "$CLI_FORCE" == "true" ]]; then
        log_debug "mode=cron (fresh install, --force, no prompt)"
        echo "cron"
        return
    fi
    log_debug "mode=prompt (fresh interactive install)"
    prompt_mode
}

# resolve_version picks an explicit tag or falls back to "latest". Logs
# the decision under --verbose.
resolve_version() {
    if [[ -n "$CLI_VERSION" ]]; then
        log_info "Using requested release: ${CLI_VERSION}"
        verify_release_tag "$CLI_VERSION"
        echo "$CLI_VERSION"
        return
    fi
    log_info "Fetching latest release tag..."
    local v
    v=$(get_latest_version)
    log_info "Latest release: ${v}"
    log_debug "version=${v} (from GitHub /releases/latest)"
    echo "$v"
}

# short_circuit_if_up_to_date exits 0 when the installed binary already
# matches the target tag AND --force isn't set. Returns without exiting
# otherwise so the install proceeds.
short_circuit_if_up_to_date() {
    local is_upgrade="$1" version="$2"
    [[ "$is_upgrade" != "true" ]] && return
    [[ "$CLI_FORCE" == "true" ]] && return

    local current
    current=$(binary_version_bare "${INSTALL_DIR}/${BINARY_NAME}")
    log_debug "short-circuit check: current=${current:-?} target=${version}"
    if [[ -n "$current" ]] && { [[ "$current" == "$version" ]] || [[ "v$current" == "$version" ]]; }; then
        log_success "dddns ${version} already installed — nothing to do"
        exit 0
    fi
}

# run_install is the install/upgrade happy path. Called after argument
# parsing, root + platform checks, and (for upgrades) mode preservation.
run_install() {
    local is_upgrade="false"
    has_config && is_upgrade="true"
    log_debug "is_upgrade=${is_upgrade}"

    local mode
    mode=$(resolve_mode "$is_upgrade")

    gather_environment "${CLI_VERSION}" "$mode"
    print_environment

    if [[ "$CLI_FORCE" != "true" ]] && [[ "$is_upgrade" != "true" ]]; then
        local response
        response=$(prompt_tty "Proceed? [Y/n]: " "Y")
        if [[ "$response" =~ ^[Nn] ]]; then
            log_info "Installation cancelled"
            exit 0
        fi
        echo ""
    fi

    ensure_on_boot_hook

    local version
    version=$(resolve_version)
    short_circuit_if_up_to_date "$is_upgrade" "$version"

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

    print_success "$is_upgrade" "$mode"
}

print_success() {
    local is_upgrade="$1" mode="$2"
    if [[ "$is_upgrade" == "true" ]]; then
        print_banner "Upgrade complete (mode=${mode})"
    else
        print_banner "Install complete (mode=${mode})"
    fi
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

main() {
    parse_args "$@"

    check_root

    if [[ "$CLI_ACTION" == "rollback" ]]; then
        rollback_action
        exit 0
    fi

    detect_arch
    detect_unifi_device
    check_prerequisites

    case "$CLI_ACTION" in
        probe)     probe_command; exit 0 ;;
        uninstall) uninstall;     exit 0 ;;
        install)
            print_banner "dddns Installer for UniFi Dream"
            run_install
            ;;
    esac
}

main "$@"
