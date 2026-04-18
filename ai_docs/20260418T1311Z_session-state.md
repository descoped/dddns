# Session State — v0.2.0 Release Prep

Snapshot for session handoff / context compaction.

**Branch:** `main` at `3c9518d`. Working tree clean. All work pushed.

## What landed this session

Six commits on `main`, chronological:

| Commit | Subject |
|---|---|
| `cbc8d22` | refactor: retire AWS SDK and viper; stdlib-only deps |
| `83fc713` | docs: add release-prep plan |
| `347a1cb` | docs: design "dddns install" for zero-question platform bootstrap |
| `a85c3e5` | chore: add environment-report issue template |
| `6587bc9` | feat(installer): add pre-flight, state snapshot, smoke test, rollback |
| `3c9518d` | test: scrub environment-specific fixtures to RFC placeholders |

### Headline metrics at HEAD

- **Binary:** 16 MB → **7.84 MB stripped** (`CGO_ENABLED=0 go build -ldflags "-s -w"`).
- **Direct deps:** 5 → **2** (`cobra`, `yaml.v3`). Indirect: 13 → 4.
- **Tests:** 223 pass under `go test -race ./...` across 15 packages.
- **No AWS SDK, no viper.** Verified by `grep -rn "aws-sdk\|viper"` returning zero hits in `.go`/`go.mod`/`go.sum`.
- **No environment-specific hardcodings** in production code. Test fixtures use RFC-reserved placeholders (`203.0.113.42`, `home.example.com`) via per-package `testPublicIP` / `testHostname` constants.

### What each commit delivered

- **AWS SDK + viper retirement (`cbc8d22`):**
  - `internal/dns/sigv4.go` (new, 174 lines) — standalone SigV4 signer, validated against AWS's documented signing-key reference vector.
  - `internal/dns/route53.go` rewritten on `net/http` + `encoding/xml`. Public signatures unchanged (`NewRoute53Client`, `NewFromConfig`, `GetCurrentIP`, `UpdateIP`, `fqdn`).
  - `internal/config/path.go` (new) — package-level `activeConfigPath` with `SetActivePath` / `ActivePath`.
  - `internal/config/config.go` — `Load()` uses `os.ReadFile` + `yaml.Unmarshal` directly. `ForceUpdate`/`DryRun` removed from `Config` (flag-only, read from `cmd` package vars).
  - `cmd/root.go` — `initConfig()` resolves path explicitly (flag → `.secure` → `config.yaml`) and calls `config.SetActivePath`.
  - All viper-based tests migrated to `config.SetActivePath(path)` (the viper `Reset`/`SetConfigFile`/`ReadInConfig` triple collapses to one call).
  - One test deleted: `TestLoadConfigWithFlags` (was asserting viper flag binding; no longer applicable).

- **Release-prep plan (`83fc713`):** `ai_docs/20260417T2002Z_release-prep.md` — eight items: library reduction, workflow fitness, memory-leak audit, docs refresh, installer robustness, on-boot persistence + rolling logs, upgrade flow, console testability. Item 1 marked Done; items 2–8 are Planned.

- **CLI-for-idiots design (`347a1cb`):** `ai_docs/20260417T2033Z_cli-for-idiots.md` — spec for `dddns install` / `dddns doctor` / `dddns uninstall`, platform auto-detection, mode picker, 28 edge cases. Deferred to v0.3.0.

- **Environment-report template (`a85c3e5`):** `.github/ISSUE_TEMPLATE/environment-report.md` — intake form for contributors requesting platform support (copy-paste probe blocks + secret-protection rules).

- **Installer safety guards (`6587bc9`):** `scripts/install-on-unifi-os.sh` rewrite.
  - **Pre-flight:** new binary's `--version` + `config check` run against the *existing* config before anything on-disk changes.
  - **State snapshot:** binary + boot script + cron entry + systemd unit copied to `.prev` peers.
  - **Post-install smoke:** `--version` + `config check` on installed binary; failure triggers auto-rollback.
  - **`--rollback` flag:** manual restore of `.prev` on demand.
  - `apply_mode` returns 0/1 cleanly — no `set -e` trap escape. Main flow can conditionally roll back.
  - Device detection reads `/proc/ubnthal/system.info` first, reports model.
  - Hard fails on missing `systemctl`, non-writable `/etc`, `<50 MB` free on `/data`.
  - Phase-prefixed log lines (`[preflight]`, `[backup]`, `[install]`, `[apply]`, `[smoke]`, `[rollback]`).
  - Shellcheck-clean.

- **Environment scrub (`3c9518d`):**
  - Real WAN IP `81.191.174.72` → `203.0.113.42` (RFC 5737 TEST-NET-3).
  - Real hostname `home.route-66.no` → `home.example.com` (RFC 2606).
  - Per-package `testPublicIP` constants extracted — 3 const declarations, 20 call-sites routed through them.
  - Production code verified clean of environment-specific hardcodings (the two `home.example.com` strings in `cmd/config.go` and `internal/config/config.go` are example text in an interactive prompt and YAML comment — RFC 2606 authorised documentation usage).

## UDR7 ground truth (captured this session, not in the repo)

Observed via SSH probes to `BRA-UDR`. Do not paste this data into any committed file — it's the user's environment and stays out of code/docs.

- Device: UDR7, Debian 11 bullseye, kernel 5.4, aarch64, systemd 247.
- `/proc/ubnthal/system.info` has `systemid=a67a`, `shortname=UDR7`.
- `systemctl`, `logger`, `crontab` all in PATH.
- `/data/on_boot.d/` present. `udm-boot.service` exists at `/etc/systemd/system/udm-boot.service`, **but** last run exit-123 (journal rotated, root cause not diagnosed — almost certainly a legacy `05-install-cni-plugins.sh` or similar script in the hook dir; *not* our `20-dddns.sh`).
- Prior working cron-mode install: `/data/dddns/dddns` (Sep 2025 AWS-SDK-era binary, 9.7 MB) → `/usr/local/bin/dddns` symlink → `/etc/cron.d/dddns` (old-format, appends to `/var/log/dddns.log`) → running every 30 min cleanly for ~7 months, cache is warm, updates are no-op'ing at 22:30.
- **Policy-based routing:** `main` routing table has **no default route**. Default lives in `201.eth4`: `default via 81.191.168.1 dev eth4 proto dhcp`. Rule 32766 (`from all lookup 201.eth4`) is the catch-all.
- WAN interface is **`eth4`** with public `/21` netmask. UDM-Pro/UDM-SE use `eth8`/`eth9`; UDR7 is different.
- No network namespaces; no PPPoE.

**Design implication:** `wanip.AutoDetect` must fall back to "first interface with a publicly-routable IPv4" when `/proc/net/route` (which reads `main` only) finds no default route. On UDR7 this picks `eth4` cleanly; on UDM-family / Pi / generic Linux, the `main`-table default-route path still wins.

## Remaining work for v0.2.0

Ordered:

1. **`wanip` auto-detect public-IP fallback.** ~30 LoC + 2 tests. Scan all interfaces via the existing `interfaceAddrs` hook when the route-file parse returns no default. Blocks UDR7 (and any future UniFi variant with policy-based routing).

2. **Tag `v0.2.0-rc.1`.** GoReleaser publishes the binary + `checksums.txt`. Required so the safety-guarded installer can actually fetch the new artefact.

3. **On-device UDR7 validation.** **Blocked on user-supplied SSH.** User asked for SSH info twice in this session; not yet provided. Acceptable to defer to after v0.2.0-rc.1 tag (the safety guards make re-trying low-risk).

4. **Tag `v0.2.0`.** Once RC looks clean on UDR7.

## Deferred to v0.3.0

Per `ai_docs/20260417T2033Z_cli-for-idiots.md`:

- `dddns install` (platform-auto, mode-auto, self-installing)
- `dddns doctor` (diagnostic)
- `dddns uninstall` (wraps removal steps)
- Shell installer shrinks to ~40 lines (`curl | tar | mv && dddns install`)
- `internal/install/` package
- `internal/profile.Scheduler()` for `{systemd, cron, launchd, none}` dispatch
- `internal/bootscript` gains `renderSystemdTimer` / `renderLaunchd`

Per `ai_docs/20260417T2002Z_release-prep.md` items 2–8:

- CI workflow polish: `-trimpath` in `build-test` + `.goreleaser.yaml`, lint-gate (flip `continue-on-error: false`), concurrency block on main/PRs.
- Cron-mode log routing through journald (`| logger -t dddns`) instead of `>> /var/log/dddns.log`.
- Docs refresh (`docs/*.md` sweep against current command surface — already grep-clean of `skip_proxy_check` / `IsProxyIP` / `--check-proxy`).

## Open items / waiting on user

- **UDR7 SSH details** for on-device validation. Asked; still pending.
- **Go / no-go on Path A** (minimal v0.2.0 = `wanip` fallback + tag) vs **Path B** (bundle the big `dddns install` rewrite into v0.2.0). Recommendation in the last response: Path A.

## Key artifacts on `main`

| Path | Purpose |
|---|---|
| `ai_docs/20260417T1558Z_code-review.md` | Full codebase code review. All 17 findings applied. |
| `ai_docs/20260417T2002Z_release-prep.md` | Eight-item release plan. Status: Partial (item 1 done). |
| `ai_docs/20260417T2033Z_cli-for-idiots.md` | v0.3.0 design for `dddns install` / `doctor` / `uninstall`. |
| `ai_docs/20260418T1311Z_session-state.md` | This file. |
| `ai_docs/0–5_*.md` | Forward-looking plans. Each opens with Status / Confidence / Last-reviewed. |
| `.github/ISSUE_TEMPLATE/environment-report.md` | Contributor intake form. |
| `scripts/install-on-unifi-os.sh` | Safety-guarded installer (pre-flight / snapshot / smoke / rollback / `--rollback`). |

## How to resume

1. `git fetch origin && git status` — confirm `main` at `3c9518d` or later.
2. Read `ai_docs/20260417T2002Z_release-prep.md` (eight-item plan) and this file.
3. Start the `wanip` public-IP fallback: modify `internal/wanip/wanip.go` `autoDetect` path to scan all interfaces via the existing `interfaceAddrs` hook when the `/proc/net/route` parse finds no default. Add two tests — one simulating UDR7 (no default, eth4 has public IP) and one regression test that the default-route path still wins on a normal platform.
4. After `wanip` lands, tag `v0.2.0-rc.1`. GoReleaser runs on the tag and publishes the new binary + `checksums.txt`.
5. If/when UDR7 SSH is available: run `bash scripts/install-on-unifi-os.sh` on the device. The safety guards mean the worst case is "rolled back to today's working state". Verify with `dddns config check`, `dddns update --dry-run`, `tail /var/log/dddns.log` after the next cron.
6. Tag `v0.2.0` when the RC is green on UDR7.

## Out-of-repo context that matters

- **`/Users/oranheim/Code/script-utils/`** is the user's separate shell-utilities repo. This session audited its `installer/install.sh` and wrote `REQUIREMENTS.md` there documenting ten hard rules + seven current gaps. User confirmed they fixed + pushed. Not in dddns scope, but informs the "opinionated, no hardcoding" bar the user holds across all their projects.
- **User's environment details** (hostname `BRA-UDR`, serial, MAC, activation code, `home.route-66.no`, `81.191.174.72`) are *never* to be committed to this repo. The earlier scrub (`3c9518d`) verified production is clean; tests use RFC-reserved placeholders. Treat this as a permanent rule, not a one-time cleanup.
