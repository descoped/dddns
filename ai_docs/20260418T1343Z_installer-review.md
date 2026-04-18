# Installer review — `scripts/install-on-unifi-os.sh`

**Reviewed:** 2026-04-18
**Scope:** health + verbose logging / debuggability for RC test cycles
**Status:** Report only — nothing applied
**Lines reviewed:** 718, single-file
**Findings:** 12 (2 critical, 5 refactor, 5 minor)

The script is already shellcheck-clean and `bash -n` clean, with solid safety structure: preflight → snapshot → install → apply → smoke, with rollback on any downstream failure. The findings below target two real gaps: a few misleading error paths that will cost time during RC iteration, and the absence of a verbose / debug mode for the phases that currently run in silent `>/dev/null 2>&1` form.

---

## Critical (2)

### C1. `curl` in `download_binary` lacks `-f` — 404 masquerades as SHA mismatch
**Location:** line 172 `curl -L -o "${temp_dir}/${archive_name}" "${base_url}/${archive_name}" --progress-bar`

Without `--fail`, curl writes GitHub's 404 HTML page to the tarball path and returns 0. The next step computes SHA-256 of the HTML, finds it doesn't match, and errors with:

> `[download] SHA-256 mismatch — binary tampered or corrupted`

This is exactly wrong for the RC-test scenario. A mistyped tag, a build that hasn't finished, or an asset name mismatch all trigger this misleading error. Fix: add `-f`:

```bash
curl -fL -o "${temp_dir}/${archive_name}" "${base_url}/${archive_name}" --progress-bar
```

Then 404 surfaces as `[download] Failed to fetch …` with the HTTP status visible.

### C2. `apply_mode` secret capture mixes stdout and stderr
**Location:** line 419 `secret=$("${dddns}" config rotate-secret --init --quiet 2>&1)`

The intent is "capture the secret, if the command fails use `$secret` as the error message". But `2>&1` means any WARN / deprecation notice emitted to stderr (even under `--quiet`) ends up interleaved with the secret, which is then printed verbatim in the UniFi UI block (line 462). A user who pastes a polluted secret into the UI will get 401s for reasons that look unrelated.

Fix: split capture + error handling:

```bash
local err
secret=$("${dddns}" config rotate-secret --init --quiet 2>/tmp/dddns-rotate-secret.err)
if [[ $? -ne 0 ]]; then
    err=$(cat /tmp/dddns-rotate-secret.err)
    rm -f /tmp/dddns-rotate-secret.err
    log_error "[apply] Failed to initialize serve-mode secret: ${err}"
    return 1
fi
rm -f /tmp/dddns-rotate-secret.err
```

Or better — make `rotate-secret --init --quiet` guarantee stderr-only-on-error (already the intent of `--quiet`) and keep it as `2>/dev/null`.

---

## Refactor (5)

### R1. Add `--verbose` / `DDDNS_DEBUG=1` for the silent phases
Currently `bash "${BOOT_SCRIPT}" >/dev/null 2>&1` (line 435), plus four `systemctl`/`cron` restart calls (lines 294–297, 483), all swallow output unconditionally. When a test build misbehaves, the first question is "what did the boot script print?" and there's no way to see it without editing the installer on the device.

Pattern:
```bash
# top of file
VERBOSE="${DDDNS_DEBUG:-0}"

# arg parsing
--verbose|-v) VERBOSE=1; shift ;;

# redirection helper
vexec() {
    if [[ "$VERBOSE" == "1" ]]; then
        "$@"
    else
        "$@" >/dev/null 2>&1
    fi
}

# usage site
vexec bash "${BOOT_SCRIPT}" || log_warning "[apply] Boot script returned non-zero"
vexec /etc/init.d/cron restart || true
vexec systemctl daemon-reload || true
```

When RC testing on UDR7, `DDDNS_DEBUG=1 bash scripts/install-on-unifi-os.sh --version v0.2.0-rc.1` surfaces every underlying call.

### R2. Replace `set -e` with `set -euo pipefail`
**Location:** line 27

`set -u` catches unset-variable typos before they silently produce empty strings. `set -o pipefail` catches failures anywhere in a pipeline, not just the last stage. Both are the standard modern default. No known problem sites today, but turning them on lowers the cost of future modifications.

Two sites need attention after the flip:
- `available=$(df -BM /data 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/M//')` — if `df` fails, the pipeline as a whole returns non-zero. The current `[[ -n "$available" ]]` guard already handles the empty case, but wrap in `|| true` for clarity.
- `grep + sed` parse in `get_latest_version` — already explicit empty-check, works fine with pipefail.

### R3. Capture `config check` output once, not twice
**Location:** lines 249–253 in `preflight_binary`, 334–338 in `postinstall_smoke`

```bash
if ! "${tb}" config check >/dev/null 2>&1; then
    "${tb}" config check 2>&1 | sed 's/^/    /' >&2
    return 1
fi
```

Running the command twice wastes a second process invocation and — worse — opens a small non-determinism window if `config check` ever grew a side effect. One-shot capture:

```bash
local out
if ! out=$("${tb}" config check 2>&1); then
    log_error "[preflight] New binary rejected existing config:"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    return 1
fi
log_success "[preflight] Config loads cleanly"
```

### R4. Print an "Environment:" block at the top of `main`
Every RC bug report comes with "what device, what arch, what mode, what tag". Dumping a one-shot block immediately after the banner makes those reports self-contained:

```
Environment:
  • Device:       UDR7 (/proc/ubnthal/system.info: shortname=UDR7)
  • Arch:         aarch64 → arm64
  • Mode target:  cron (preserved from existing install)
  • Version:      v0.2.0-rc.1 (requested via --version)
  • Install dir:  /data/dddns
  • Config:       /data/.dddns/config.yaml (exists)
  • Bootscript:   /data/on_boot.d/20-dddns.sh (exists)
  • Log file:     /var/log/dddns.log (exists, 1.2M)
  • Free on /data: 4820M
```

This replaces the current scattered log lines (`[INFO] Detected ARM64 architecture`, `[INFO] Upgrade detected — preserving mode: cron`, etc.) with one block that can be copy-pasted into an issue.

### R5. Print `--version` output on rollback completion
**Location:** `rollback_action`, line 507

`"${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || true` shows the NEW (restored) binary's version. The user also wants to see what was replaced — include a delta line:

```bash
log_success "[rollback] Complete"
log_info "  Now running: $(...)"
log_info "  Previously:  (snapshotted .prev is now live; the replaced build was overwritten)"
```

Even better: before rolling back, `file`-stat the `.prev` peer's mtime so we can say "restored to state from 2026-04-18 13:07".

---

## Minor (5)

### M1. `temp_dir` should use `mktemp -d`
**Location:** line 187 `temp_dir="/tmp/dddns-install-$$"`

`$$` avoids parallel collision within different PIDs but doesn't guarantee the directory is fresh. `mktemp -d -t dddns-install.XXXXXXXX` is the idiomatic choice and yields `/tmp/dddns-install.a1B2c3D4` — random suffix, guaranteed fresh, cleaned up by `trap rm -rf`.

### M2. `curl` progress-bar in non-TTY context
**Location:** line 172 `curl -L … --progress-bar`

When invoked via `curl … | bash` the progress bar is emitted into a non-TTY and produces line-noise. Use `[[ -t 1 ]] && PROGRESS="--progress-bar" || PROGRESS="--silent --show-error"`, then `curl -fL $PROGRESS …`.

### M3. Upgrade short-circuit is brittle
**Location:** line 654 `current=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | awk '{print $3}' || echo "")`

Position-3 parsing assumes the version output format `dddns version X.Y.Z (commit date)`. If that format changes, the short-circuit silently fails and re-downloads — safe failure, but wasteful. Consider `dddns --version` emitting a machine-readable form (`dddns --version --short` → just `v0.2.0-rc.1`), or parse with a tolerant regex.

### M4. `ensure_on_boot_hook` pipes curl to bash without verification
**Location:** line 142

The dddns binary itself is SHA-256 verified (C1 fix will make this clearer), but the upstream `unifi-utilities/unifios-utilities` install script is piped to bash without a hash check. There's no published checksum upstream, so this is an accepted supply-chain tradeoff. Worth a visible log line so it's not a surprise:

```
[on-boot] Installing unifios-utilities (no checksum published upstream — supply-chain trust assumed).
```

### M5. Phase timing for profiling
**Location:** `log_phase` implementation

For profiling test cycles, swap:
```bash
log_phase() { echo -e "${BLUE}[$1]${NC} $2"; }
```
for:
```bash
log_phase() {
    local now
    now=$(date +%H:%M:%S)
    echo -e "${BLUE}[${now}] [$1]${NC} $2"
}
```
Or, for phase deltas, record `SECONDS` at each phase entry and print the delta at exit.

---

## Recommendation for the immediate v0.2.0-rc.1 iteration

**Do now before tagging:** C1 (curl `-f`) and C2 (secret capture). Both are one-line fixes that directly affect RC debugging.

**Do after rc.1 if it goes smoothly:** R1 (verbose flag), R3 (one-shot config check capture), R4 (environment block). These raise the floor for every future RC cycle.

**Defer to v0.3.0:** R2, R5, all M-tier. Nice hygiene, not RC-blocking.

---

## What's already good (don't touch)

- Three-gate safety (preflight / snapshot / smoke) with rollback is the correct model.
- `.prev` snapshot uses `cp -a` and `mv` for atomic restore — correct.
- SHA-256 verification against the GoReleaser-published `checksums.txt` — correct trust anchor.
- Device detection via `/proc/ubnthal/system.info` with documented fallback — matches upstream behavior.
- `apply_mode` delegates bootscript generation to the binary rather than open-coding systemd units in shell — correct separation of concerns.
- Mode preservation on upgrade + `prompt_mode` on fresh install — good UX.
- `--rollback` is a first-class command, not an undocumented flag — correct for the "oh no" case.
