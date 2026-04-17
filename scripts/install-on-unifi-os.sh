#!/bin/bash
#
# dddns installer for Ubiquiti UniFi Dream devices
#
# Usage:
#   curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
#   ./install-on-unifi-os.sh [--mode cron|serve] [--force] [--uninstall]
#
# Modes are mutually exclusive:
#   cron  — /etc/cron.d/dddns runs `dddns update` every 30 minutes (default).
#   serve — /data/on_boot.d/20-dddns.sh starts a supervised `dddns serve`
#           loop that handles dyndns requests from the UniFi UI.
#
# On existing installs the script preserves the currently-configured
# mode unless --mode is passed explicitly.

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

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error()   { echo -e "${RED}[✗]${NC} $1" >&2; }
log_warning() { echo -e "${YELLOW}[!]${NC} $1"; }

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

check_udm() {
    if [[ ! -d "/data" ]]; then
        log_error "/data not found — this does not look like a UniFi Dream device"
        exit 1
    fi
    if [[ -f /etc/unifi-os/unifi-os.conf ]]; then
        log_info "Detected UniFi OS v3"
    elif [[ -d /etc/unifi-core ]] || [[ -f /etc/default/unifi ]]; then
        log_info "Detected UniFi OS v4"
    elif [[ -f /etc/board.info ]] || [[ -d /data/unifi ]]; then
        log_info "Detected Ubiquiti device"
    else
        log_warning "Could not confirm UniFi OS version, but /data exists — continuing"
    fi

    local available
    available=$(df -BM /data | awk 'NR==2 {print $4}' | sed 's/M//')
    if [[ $available -lt 50 ]]; then
        log_warning "Low disk space: ${available}MB on /data (50MB recommended)"
    else
        log_success "Disk space on /data: ${available}MB"
    fi
}

install_unifios_utilities() {
    if [[ ! -d "/data/on_boot.d" ]] && [[ ! -f "/data/on_boot.sh" ]]; then
        log_warning "on-boot-script not installed — required for persistence across firmware updates"
        echo ""
        echo -n "Install unifios-utilities now? [Y/n]: "
        read -r response </dev/tty || response="y"
        if [[ -z "$response" ]] || [[ "$response" =~ ^[Yy] ]]; then
            log_info "Installing unifios-utilities..."
            curl -fsL "https://raw.githubusercontent.com/unifi-utilities/unifios-utilities/HEAD/on-boot-script/remote_install.sh" | bash || {
                log_error "Failed to install unifios-utilities (boot persistence will not work)"
            }
        else
            log_warning "Skipping — dddns may not persist across firmware updates"
        fi
    fi
    mkdir -p "${BOOT_SCRIPT_DIR}"
}

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

# Download binary + checksums.txt, verify SHA-256, extract, install.
install_binary() {
    local version="$1"
    local force="${2:-false}"

    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]] && [[ "$force" != "true" ]]; then
        local current
        current=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $3}')
        if [[ "$current" == "$version" ]] || [[ "v$current" == "$version" ]]; then
            log_success "dddns ${version} already installed"
            return 0
        fi
    fi

    local archive_name="dddns_Linux_${ARCH}.tar.gz"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${version}"
    local temp_dir="/tmp/dddns-install-$$"
    mkdir -p "${temp_dir}"
    # shellcheck disable=SC2064
    trap "rm -rf '${temp_dir}'" EXIT

    log_info "Downloading ${archive_name}..."
    if ! curl -L -o "${temp_dir}/${archive_name}" "${base_url}/${archive_name}" --progress-bar; then
        log_error "Failed to download ${archive_name}"
        exit 1
    fi

    log_info "Fetching checksums.txt for SHA-256 verification..."
    if ! curl -fsL -o "${temp_dir}/checksums.txt" "${base_url}/checksums.txt"; then
        log_error "Could not fetch checksums.txt — refusing to install an unverified binary"
        exit 1
    fi

    local expected
    expected=$(awk -v name="${archive_name}" '$2 == name {print $1}' "${temp_dir}/checksums.txt")
    if [[ -z "$expected" ]]; then
        log_error "${archive_name} not listed in checksums.txt"
        exit 1
    fi
    local actual
    actual=$(sha256sum "${temp_dir}/${archive_name}" | awk '{print $1}')
    if [[ "${expected}" != "${actual}" ]]; then
        log_error "SHA-256 mismatch — binary tampered with or corrupted"
        log_error "  Expected: ${expected}"
        log_error "  Got:      ${actual}"
        exit 1
    fi
    log_success "Binary SHA-256 verified"

    log_info "Extracting..."
    tar -xzf "${temp_dir}/${archive_name}" -C "${temp_dir}" || {
        log_error "Failed to extract ${archive_name}"
        exit 1
    }
    if [[ ! -f "${temp_dir}/${BINARY_NAME}" ]]; then
        log_error "Binary ${BINARY_NAME} not present inside ${archive_name}"
        exit 1
    fi

    mkdir -p "${INSTALL_DIR}"
    chmod +x "${temp_dir}/${BINARY_NAME}"
    mv "${temp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "/usr/local/bin/${BINARY_NAME}"
    log_success "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# Write a minimal config.yaml for fresh installs. Existing configs are
# left alone so upgrades don't clobber user settings.
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

# Detect the mode of the existing install by reading the generated boot
# script's mode marker. Prints "cron", "serve", or empty string.
detect_current_mode() {
    [[ -f "${BOOT_SCRIPT}" ]] || { echo ""; return; }
    if grep -q "^# --- serve mode ---" "${BOOT_SCRIPT}"; then
        echo "serve"
    elif grep -q "^# --- cron mode ---" "${BOOT_SCRIPT}"; then
        echo "cron"
    elif [[ -f "${CRON_FILE}" ]]; then
        # Pre-E1 installs wrote /etc/cron.d/dddns inline.
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

# apply_mode delegates boot-script generation to the binary and then
# runs the script once so the install is effective without a reboot.
apply_mode() {
    local mode="$1"
    local dddns="${INSTALL_DIR}/${BINARY_NAME}"
    local secret=""

    if [[ "$mode" == "serve" ]]; then
        log_info "Initializing serve-mode shared secret..."
        if ! secret=$("${dddns}" config rotate-secret --init --quiet); then
            log_error "Failed to initialize serve-mode secret"
            exit 1
        fi
    fi

    log_info "Generating boot script (mode=${mode})..."
    "${dddns}" config set-mode "${mode}" --boot-path "${BOOT_SCRIPT}" >/dev/null

    log_info "Applying boot script..."
    # The boot script is idempotent — re-running it switches away from
    # the other mode's artefacts as needed.
    bash "${BOOT_SCRIPT}" || log_warning "Boot script returned non-zero — check ${LOG_FILE}"

    if [[ "$mode" == "serve" ]]; then
        print_unifi_ui_values "$secret"
    fi
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
    echo "  UniFi UI values"
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
    echo "The secret above is written to config (encrypted if you run"
    echo "'dddns secure enable') and will not be shown again. To rotate,"
    echo "run 'dddns config rotate-secret' and update the UniFi UI."
    echo ""
}

uninstall() {
    log_warning "Uninstalling dddns..."
    rm -f "${CRON_FILE}"
    /etc/init.d/cron restart >/dev/null 2>&1 || true
    pkill -f "dddns serve" >/dev/null 2>&1 || true
    rm -f "${BOOT_SCRIPT}"
    rm -f "/usr/local/bin/${BINARY_NAME}"
    rm -rf "${INSTALL_DIR}"
    log_warning "Configuration preserved at ${CONFIG_DIR}"
    log_info "To remove configuration: rm -rf ${CONFIG_DIR}"
    log_success "dddns uninstalled"
}

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --mode cron|serve   Install or switch to the specified mode. Default:
                      preserve current mode on upgrade; prompt on fresh
                      install.
  --force             Reinstall the binary even if the current version
                      matches the latest release.
  --uninstall         Remove dddns. Preserves configuration.
  --help              Show this message.
EOF
}

main() {
    local action="install"
    local force="false"
    local mode=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --uninstall) action="uninstall"; shift ;;
            --force)     force="true"; shift ;;
            --mode)
                shift
                [[ $# -eq 0 ]] && { log_error "--mode requires an argument"; exit 1; }
                mode="$1"
                shift
                ;;
            --mode=*)    mode="${1#*=}"; shift ;;
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
    detect_arch
    check_udm

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

    install_unifios_utilities

    log_info "Fetching latest release tag..."
    local version
    version=$(get_latest_version)
    log_info "Latest release: ${version}"

    install_binary "${version}" "${force}"
    create_default_config
    apply_mode "${mode}"

    echo ""
    echo "======================================"
    if [[ "$is_upgrade" == "true" ]]; then
        echo "  Upgrade complete (mode=${mode})"
    else
        echo "  Install complete (mode=${mode})"
    fi
    echo "======================================"
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
