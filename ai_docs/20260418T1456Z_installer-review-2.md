# Installer deep review — SRP, DRY, and probing extension

**Reviewed:** 2026-04-18
**Scope:** `scripts/install-on-unifi-os.sh` at HEAD (797 LoC)
**Focus:** Single-Responsibility, Don't-Repeat-Yourself, add a probing mode
**Status:** Report only — nothing applied

Prior review (`20260418T1343Z_installer-review.md`) fixed correctness gaps (C1/C2) and debuggability (R1/R3/R4). This pass looks at the resulting structure and asks: are the pieces cleanly separated, is knowledge represented once, and can the same script self-diagnose a broken install without running an install?

Findings: 5 SRP, 6 DRY, 3 minor, 1 new feature (probe mode).

---

## SRP Violations (5)

| # | Location | Symbol | Issue | Suggestion |
|---|---|---|---|---|
| S1 | line 109–148 | `detect_unifi_device` | Mixes four concerns: device identification, `/data` presence, `systemctl` availability, `/etc` writability, disk space. | Split to `detect_unifi_device` (identity only) + `check_prerequisites` (capabilities + resources). |
| S2 | line 622–794 | `main` | 172 lines mixing arg-parse, banner, dispatch, mode resolution, environment dump, prompt, bootstrap, version resolution, short-circuit, install flow, success summary. | Split to `parse_args`, `resolve_mode`, `run_install`, `print_success`. `main` becomes a 30-line orchestrator. |
| S3 | line 440–477 | `apply_mode` | Branches into two different flows (serve needs secret init + UI values; cron just writes bootscript), each with its own inline setup. | Extract `init_serve_secret` → returns secret or fails. Keep `apply_mode` as the dispatcher. |
| S4 | line 202–255 | `download_binary` | Does fetch + verify + extract + sets output variable. Four steps in one 54-line function. | Split: `fetch_release_artifacts` (curl both files), `verify_sha256` (single-arg helper), `extract_binary`. Main flow composes them. |
| S5 | line 516–555 | `print_environment` | Gathers state AND formats output. Can't be reused by a probe command because there's no way to get the raw state without triggering printing. | Split: `gather_environment` populates an associative array; `print_environment` consumes it. Probe mode reuses the gatherer. |

---

## DRY Violations (6)

### D1. Config-presence check duplicated 4×

Same predicate at lines 273, 360, 379, 680:

```bash
if [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; then
```

Extract:
```bash
has_config() { [[ -f "${CONFIG_DIR}/config.yaml" ]] || [[ -f "${CONFIG_DIR}/config.secure" ]]; }
```

Used at all four sites. The "pick yaml vs secure for display" logic in `print_environment` (lines 529–533) stays separate — it's a different question (which one, not whether).

### D2. Snapshot file list duplicated

Hardcoded list appears in both `save_state` (line 295) and `rollback_state` (line 315):

```bash
for f in "${INSTALL_DIR}/${BINARY_NAME}" "${BOOT_SCRIPT}" "${CRON_FILE}" "/etc/systemd/system/dddns.service"; do
```

If a new artefact (e.g. a timer unit, a logrotate drop-in) joins the install, it must be added in two places or silently not snapshotted. Extract:

```bash
readonly STATE_FILES=(
    "${INSTALL_DIR}/${BINARY_NAME}"
    "${BOOT_SCRIPT}"
    "${CRON_FILE}"
    "/etc/systemd/system/dddns.service"
)
```

Both loops iterate `"${STATE_FILES[@]}"`.

### D3. Binary `--version` invocation repeated 4× with two different parse styles

- Line 271: `"${tb}" --version 2>/dev/null | head -1` (preflight, display)
- Line 358: `"${b}" --version 2>/dev/null | head -1` (smoke, display)
- Line 587: `"${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null` (rollback, display)
- Line 732: `"${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $3}'` (upgrade short-circuit, compare)

Two helpers:

```bash
binary_version_line() { "$1" --version 2>/dev/null | head -1; }   # for logging
binary_version_bare() { "$1" --version 2>/dev/null | awk '{print $3}'; }  # for compare
```

### D4. Banner pattern repeated

Banner at 658–662 and success block at 767–774 both hand-print `===` bars:

```bash
echo "======================================"
echo "  dddns Installer for UniFi Dream"
echo "======================================"
```

Extract `print_banner "title"`. Saves ~6 lines and prevents drift if someone changes the separator width.

### D5. `config check` capture pattern duplicated

`preflight_binary` (lines 273–284) and `postinstall_smoke` (lines 360–368) contain nearly identical blocks:

```bash
local check_out
if ! check_out=$("${tb}" config check 2>&1); then
    log_error "[${phase}] ${binary_label} rejected config:"
    printf '%s\n' "$check_out" | sed 's/^/    /' >&2
    return 1
fi
log_success "[${phase}] Config ..."
```

Extract `validate_config <binary-path> <phase-label>` returning 0/1.

### D6. Interactive prompt with `/dev/tty` fallback is written twice

- `prompt_mode` (lines 422–435): mode selection
- `main` inline (lines 704–712): proceed confirmation

Both follow the same "read from /dev/tty with fallback" pattern. Extract:

```bash
prompt_tty() {
    local prompt="$1" default="$2" reply
    if [[ ! -r /dev/tty ]]; then echo "$default"; return; fi
    echo -n "$prompt" >&2
    read -r reply </dev/tty || reply="$default"
    echo "${reply:-$default}"
}
```

Callers become one line.

---

## Minor (3)

### M1. `set -e` → `set -euo pipefail`

Flagged in the prior review (R2). Still not applied. `set -u` catches unset-variable typos; `set -o pipefail` catches failures anywhere in a pipe (e.g. a broken `curl | grep | sed` chain silently producing empty output).

### M2. `log_debug` is defined but only triggered by `vexec`

Line 65 defines it; line 73 uses it. No other site calls `log_debug` directly. Either remove (if `vexec` is the sole consumer, inline), or use at more decision points under `--verbose` (e.g. "chose mode=cron because boot-script has legacy cron marker"). My preference: keep it, add 3–5 well-placed `log_debug` calls in mode-detection and version-resolution so `--verbose` tells the story.

### M3. Hardcoded `/etc/init.d/cron restart` path

Lines 321, 566. UniFi OS ships cron as a SysV-style init script, not a systemd unit, so this works today. But the system also has `systemctl`, so `systemctl reload cron` would be more portable if UniFi ever migrates the cron package. Not urgent — UniFi's current layout is stable.

---

## New feature — `--probe` mode (privacy-safe self-diagnosis)

**Motivation.** During this session the user had to paste 7 manual probe commands over SSH to confirm the existing dddns install's shape. That work is mechanical and repeatable. The installer should carry it as a first-class subcommand, producing output safe to paste in an issue tracker.

**Design.**

```
./install-on-unifi-os.sh --probe
```

Exits 0 without changing any state. Output is one block with labeled sections, max 60 lines, no WAN IP, no log-body text, no config values. Safe-to-paste bar: anything in the probe output can go in a public issue.

**Section layout:**

```
=== dddns probe ===
generated: 2026-04-18T14:56:00Z
installer: 20260418/main.sh v?

[system]
  device-model: UDR7                   # /proc/ubnthal/system.info shortname
  kernel:       5.4.213
  systemd:      247
  arch:         aarch64
  firmware:     4.2.8                  # from /etc/os-release or similar

[disk]
  /data free:   4820M
  /etc writable: yes

[scheduler]
  cron service:   running              # systemctl is-active cron (or init.d status)
  cron entry:     present               # /etc/cron.d/dddns: exists (size 123 bytes)
  cron schedule:  */30 * * * *          # the schedule only, command body redacted
  on_boot.d:      3 scripts             # list names + sizes, NOT contents
    - 05-install-cni-plugins.sh (5145 bytes, 2025-03-04)
    - 06-cni-bridge.sh (140 bytes, 2025-03-04)
    - 20-dddns.sh (1252 bytes, 2025-09-14)

[dddns install]
  binary path:     /data/dddns/dddns → /usr/local/bin/dddns
  binary version:  0.1.1 (2025-09-13T22:39:13Z)
  binary arch:     ELF 64-bit LSB aarch64
  config:          config.secure (encrypted, 292 bytes)
  cache file:      last-ip.txt present (mtime 2026-04-18T13:30)   # mtime only, not value

[systemd units]
  dddns.service:   (absent)

[network (metadata only)]
  default-route interface: (none — policy-based routing in effect)
  public-IPv4 interfaces:  1            # count, not IPs or names
  CGNAT detected:          no

[scrub assertion]
  no WAN IP, hostname, or config value is in this output.
```

**Privacy enforcement rules (hard-coded in the probe code):**

1. Never `cat` a user file. Only `ls`, `stat`, `wc -l`, `du`, `file`.
2. Never print any IPv4 from `ip addr show`. Count interfaces that pass `isPublicIPv4`, do not name them.
3. For the cron entry, parse and print the *schedule field* (first 5 tokens) and the fact that there's a command, but not the command body (it contains paths and possibly comments).
4. Config files: presence + size + mtime only. Never `grep` even for key names — key names leak hostname comments etc. in this installer's template.
5. Log file: size + mtime only. Never `tail`. Log lines contain the WAN IP.

**Integration with install flow:** reuse the same probe implementation to populate `print_environment`. That collapses into S5 (split gatherer from printer). Install-mode dumps the probe block as the "Environment:" preamble.

**CLI surface:**
```
--probe              Self-diagnosis. Prints metadata (no IPs, no config
                     values, no log contents). Exits without installing.
                     Output is safe to paste in a GitHub issue.
```

Footprint estimate: ~80–100 lines of new shell in a `probe_install()` function plus small helpers. Fits in the existing file structure without needing a separate script.

---

## Recommended ordering

**Pre-RC iteration (do now — low-risk, high leverage):**
1. **D1** (`has_config`) + **D2** (`STATE_FILES` array) + **D3** (version helpers) + **D5** (`validate_config`). Four small extractions eliminate most knowledge duplication.
2. **S1** (split `detect_unifi_device`). Enables the probe to skip the identity check while still running the capability checks — probe should run anywhere on a UniFi device, even without the device being "our target".
3. **S5** (`gather_environment` vs `print_environment`) — prerequisite for probe mode.
4. **New `--probe` subcommand** — built on top of (1)–(3).

**Post-RC (do before v0.2.0 final):**
5. **S2** (`main` split). Mostly cosmetic; improves readability.
6. **D4** (`print_banner`) + **D6** (`prompt_tty`). Small, safe.
7. **M1** (`set -euo pipefail`). Audit first — check every existing pipe for silent-empty dependencies.
8. **M2** (`log_debug` expansion with 3–5 trace lines).

**Defer to v0.3.0 or skip:**
9. **S3** (apply_mode secret extraction). Marginal — function is 38 lines, already clearly split by mode.
10. **S4** (download_binary split). Marginal.
11. **M3** (cron path). Not needed until UniFi changes cron packaging.

---

## What's already good (don't touch)

- `vexec` / `VERBOSE` / `DDDNS_DEBUG` trio — clean, orthogonal, easy to extend.
- `log_phase` for user-visible progress markers + phase-prefixed errors.
- Three safety gates (preflight / snapshot / smoke) with auto-rollback on gates 2 and 3.
- `--rollback` as a first-class action, not a flag to install.
- `--version` env-var + flag, with `verify_release_tag` catching typos fast.
- Device detection via `/proc/ubnthal/system.info` with documented fallback.
- SHA-256 verification against GoReleaser's `checksums.txt` as the trust anchor.
