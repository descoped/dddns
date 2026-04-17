# Release Prep Plan

**Status:** Partial — item 1 (viper removal) shipped alongside the AWS SDK retirement; items 2–8 awaiting execution.
**Confidence:** High — all items scoped against current `main`.
**Last reviewed:** 2026-04-17

## Scope

Prepare dddns for a tagged release that will install and run persistently on UDR/UDM devices for multi-year uptimes. Cover eight concerns: library minimization, release-workflow fitness, memory-leak audit, documentation, installer robustness, on-boot persistence + rolling logs, upgrade flow, and console-testability with a token-once security model.

## Out of scope

- Multi-provider / HTTP-only provider abstraction — see `0_provider-architecture.md`. The AWS SDK is already retired; further provider work is deferred.
- Non-UniFi event detection — see `2_non-unifi-event-detection.md`.
- Further security hardening (KDF, per-install salt, passphrase) — see `4_security-roadmap.md`.

---

## 1. Library reduction — drop viper

**Current state:** `go.mod` has three direct deps after the AWS SDK retirement: `cobra`, `viper`, `yaml.v3`. Of these, `viper` pulls 14 transitive modules (`afero`, `locafero`, `mapstructure`, `conc`, `cast`, `pelletier/go-toml/v2`, `subosito/gotenv`, `sagikazarmark/locafero`, etc.). We use a narrow subset — file-path detection, YAML unmarshal via `mapstructure`, one `BindPFlag` — and none of the env-var / hot-reload / watch features viper exists for.

**Call sites:**
- `cmd/root.go:38,63-94,107-116,119-122` — config file detection, `ReadInConfig`, `ConfigFileNotFoundError` discrimination, `ConfigFileUsed`, one `BindPFlag`.
- `internal/config/config.go:12,88-124` — `ConfigFileUsed`, `IsSet`, `GetString`, `GetBool`, `Unmarshal`.

**Target:** direct YAML parsing via `gopkg.in/yaml.v3` (already in `go.mod`). Replace viper with ~60 lines of explicit config-path resolution and a single `yaml.Unmarshal` call. `mapstructure` tags become `yaml` tags on `Config` / `ServerConfig` (both sets already coexist in the struct literals — we keep `yaml`, drop `mapstructure`).

**Expected savings:**
- 14 fewer indirect modules.
- ~1–2 MB off the binary.
- Simpler config-loading code path — easier to reason about for long-lived processes.

**Risks:**
- Loss of viper's typed getters (`GetString`, `GetBool`) — but we only use them for two flag values (`force`, `dry-run`) both of which cobra pflag can supply directly to a struct.
- Loss of viper's config-path search — we roll our own (it's ~5 lines: `cfgFile || securePath || dataDir+"/config.yaml"` with explicit existence checks).

**Verification:**
- `go test -race ./...` clean.
- Config loading still handles `--config /path/to/file.yaml`, `--config /path/to/file.secure`, auto-detect from profile's data dir.
- `go.sum` has zero `viper` / `afero` / `locafero` / `mapstructure` / `conc` / `cast` / `pelletier` / `subosito` entries.

**Status:** Done. Viper removed alongside the AWS SDK retirement in the same commit. Direct deps now: `cobra`, `yaml.v3`. Stripped binary: 7.84 MB (down from 16 MB before this session).

---

## 2. GitHub workflow fitness

**Current state:**
- `.github/workflows/ci.yml` runs `test`, `lint`, `build-test` on every push/PR. Build-test matrix covers 7 platforms (linux amd64/arm64/armv7, darwin amd64/arm64, windows amd64/arm64). Hard-coded 5 MB / 15 MB size range gates each build. `continue-on-error: true` on the lint step.
- `.github/workflows/goreleaser.yml` handles tagged releases via GoReleaser.
- `.goreleaser.yaml` has `-ldflags="-s -w"`, `CGO_ENABLED=0`, multi-arch, checksums, nfpm (deb/rpm/apk).

**Findings:**

1. **Missing `-trimpath`** in both CI build-test and `.goreleaser.yaml` `builds[].flags`. Adding it removes absolute build paths from the binary (reproducible builds + smaller runtime panics). Small win.

2. **CI `build-test` doesn't use `-s -w`.** That's fine for a test build (keeps symbols for potential panic-debugging in CI logs) but means the CI size check is measuring an unstripped binary. With `-s -w` added, the CI check would measure ~1 MB less. Today's 12.7 MB stripped → ~13.7 MB unstripped. Still under 15 MB, so the gate is not currently wrong — but the CI size is not the release size. Document this or align.

3. **`lint` has `continue-on-error: true`.** Means a lint regression cannot fail CI. If we want lint to be a release gate, flip this to `false`. If we don't, remove the job from the `build-test` `needs:` list so it doesn't look load-bearing.

4. **Coverage upload has `continue-on-error: true`** — correct (codecov outages shouldn't fail our CI).

5. **Path filters exclude `docs/**` and `ai_docs/**`** — correct.

6. **No concurrency group.** Concurrent pushes to the same branch queue instead of cancelling stale runs. Low priority; add a `concurrency:` block to cancel in-flight runs per branch.

7. **`GO_VERSION: '1.25'`** — fine. Stays stable via Renovate.

**Target:**
- Add `-trimpath` to `.goreleaser.yaml` and to the CI `build-test` go build invocation.
- Flip `lint.continue-on-error` to `false` (lint becomes a gate) OR remove it from `build-test.needs`. Pick one. I'll recommend flipping to false — the bar is `golangci-lint` clean, which current code is.
- Add a `concurrency` block to cancel superseded pushes.
- Consider raising the CI size cap to 20 MB to give headroom for future feature work (or leave at 15 MB — we're currently at 12.7 MB with room to spare).

**Risks:** low. Workflow changes only.

---

## 3. Long-running memory-leak audit

**Target environment:** `dddns serve` supervised by systemd on UDM/UDR. Expected uptime: years, bounded by firmware upgrades and power events.

**Current defenses (from the earlier code review):**
- Context propagation is tight — every outbound call respects `ctx`.
- The sole goroutine in `internal/server/server.go` joins via `http.Server.Shutdown` on ctx cancel.
- Status file writes are atomic (write-to-temp + rename).
- Audit log rotates at 10 MB to `.old`.

**Surface to audit:**

1. **`internal/server/auth.go` — sliding-window lockout state.** The `Authenticator` keeps a slice of recent-failure timestamps in memory. The sliding-window implementation SHOULD trim old entries on each check, but any leak here grows per-attacker. Verify: trim logic runs on every `Check`, not just on success. Bounded memory even under sustained attack.

2. **HTTP client connection pool.** `http.DefaultClient` in the Route53 client keeps idle connections. Default `MaxIdleConnsPerHost = 2` — bounded. Low concern.

3. **`encoding/xml` decoder buffer sizes.** Route53 responses are bounded by `maxitems=1` on list calls and small response bodies on change calls. No accumulation.

4. **`internal/config/secure_config.go` — file-wipe buffer.** `make([]byte, info.Size())` is stack-allocated per call, GC'd. No leak.

5. **journald + audit log rotation.** Serve-mode logs to journald (OS-managed rotation). Audit log rotates at 10 MB. Both bounded.

6. **Cron-mode `dddns.log`.** The install script writes `>> /var/log/dddns.log 2>&1` from the crontab. **No rotation.** Over years, this grows to fill `/var/log/`. Fix: add a `logrotate` config alongside the cron entry, OR rotate from the bootscript, OR log to journald via `systemd-cat`.

**Target:**
- Read `internal/server/auth.go` and confirm the failure-tracking slice trims on every check.
- Add rotation for `/var/log/dddns.log` in cron mode (bootscript drops a `/etc/logrotate.d/dddns` file, or cron entry pipes through `logger -t dddns` to journald — which is simpler on UniFi OS).
- Document the long-running posture in `docs/udm-guide.md`: what rotates, where, and how to read logs after N months.

**Risks:** low. Bounded checks + one cron-mode log-rotation fix.

---

## 4. Documentation refresh

**Current state:** `docs/` has `aws-setup.md`, `udm-guide.md`, `troubleshooting.md`, `installation.md`, `commands.md`, `configuration.md`, `quick-start.md`, `README.md`. Last major refresh was the UniFi bridge ship (commit range `6d84ad2`–`5ed0518`).

**Drift candidates (to verify against current code):**

1. `docs/configuration.md` — may still reference `SkipProxy` / `skip_proxy_check` (retired). Grep and clean.
2. `docs/quick-start.md` — may predate serve mode. Needs a "cron vs serve" chooser at the top.
3. `docs/commands.md` — may miss `dddns config set-mode`, `dddns config rotate-secret`, `dddns serve`/`serve status`/`serve test`.
4. `docs/installation.md` — needs a top-level "pick your platform" table and a UniFi-specific "use the installer script" pointer.
5. `docs/README.md` — index page; make sure it reflects current doc set.
6. Cross-references from root `README.md` to `docs/` files — verify all links work.

**Target:**
- Grep for `skip_proxy_check`, `IsProxyIP`, `--check-proxy`, `ip-api` — all must be zero hits outside `ai_docs/` and commit history.
- Grep for `dddns daemon` (the defunct name) — zero hits.
- Each doc opens with an at-a-glance summary: who it's for + what it covers.

**Risks:** low. Docs-only changes.

---

## 5. Installer robustness across UniFi variants

**Current state:** `scripts/install-on-unifi-os.sh` (567 lines, shellcheck-clean). Handles `--mode cron|serve`, interactive mode-picker, SHA-256 checksum verification against the release `checksums.txt`, upgrade-preserves-mode via boot-script-marker parsing, uninstall via `pkill` + systemd stop. Assumes UniFi OS ≥ 2.x with systemd as PID 1.

**Variants to handle explicitly:**

| Device | OS | systemd | `/data` persistent | Notes |
|---|---|---|---|---|
| UDM | 1.x (legacy) | no | yes | Out of scope — legacy firmware. Installer should detect and exit with a clear message. |
| UDM | 2.x / 3.x | yes | yes | Primary target. |
| UDM-Pro | 2.x / 3.x | yes | yes | Same as UDM. |
| UDM-SE | 2.x / 3.x | yes | yes | Same. |
| UDM Pro Max | 3.x / 4.x | yes | yes | Same. |
| UDR | 2.x / 3.x / 4.x | yes | yes | Primary target. |
| UDR7 | 4.x | yes | yes | Latest hardware. |

**Installer must, in one pass:**

1. **Detect the device** — read `/proc/ubnthal/system.info` for `systemid=` (hardware model). Abort with a clear message on unknown models.
2. **Detect the firmware generation** — read `/etc/os-release` (`VERSION_ID` + `PRETTY_NAME`). Abort on UniFi OS 1.x (no systemd).
3. **Verify `systemctl` is present** — `command -v systemctl`. Abort if missing.
4. **Verify `/data` is writable and mounted** — `touch /data/.dddns-install-test && rm /data/.dddns-install-test`. Abort otherwise.
5. **Detect the `udm-boot` / `unifi-utilities` bootstrap.** If `/etc/systemd/system/udm-boot.service` is missing, the installer must either (a) install `unifi-utilities` first, or (b) bail with a link to the one-liner the user must run first. The `on_boot.d` pattern only works if SOMETHING re-runs those scripts on boot.
6. **Resolve `dddns` install location** — `/data/dddns/dddns` (persistent), symlink into `/usr/local/bin/` (convenient).
7. **Download the binary** — arch-specific (UDR = arm64, UDM = arm64). SHA-256 verified against `checksums.txt`.
8. **Run the mode-specific install** — systemd unit for serve, `/etc/cron.d/dddns` for cron. Both emitted by `dddns config set-mode` (already shipped).
9. **Print the exact UniFi UI values** on serve-mode install — hostname, Basic Auth password (generated, printed once).
10. **Verify end-to-end** — `dddns config check`, `systemctl is-active dddns.service` (for serve) or `test -f /etc/cron.d/dddns` (for cron). Green or abort-and-roll-back.
11. **Self-document uninstall** at the top of the installed boot script — one-liner to remove everything.

**Target:** upgrade the installer to perform all 11 checks explicitly, print a structured report, and exit with a non-zero status at the first unrecoverable failure. Add a `--diagnose` flag that runs checks 1–5 without touching anything.

**Risks:** medium. Installer is shell; failure modes in `/data` mount races or missing `udm-boot` can brick an install.

**Verification:** shellcheck clean; `bash -n`; manual runs on UDR7 (pending SSH info — see item 8).

---

## 6. On-boot persistence + rolling logs

**Current pattern:** `/data/on_boot.d/20-dddns.sh` is written by the installer; `udm-boot.service` (from the `unifi-utilities` project) runs every script in `/data/on_boot.d/` on every boot; the dddns boot script re-installs `/etc/systemd/system/dddns.service` or `/etc/cron.d/dddns`; systemd starts it. This survives firmware upgrades because `/data/` is preserved while `/etc/` is wiped.

**Reference:** `kchristensen/udm-le` has used this pattern in production for years. See their `on_boot.d` layout for precedent.

**Rolling logs — current:**
- **Serve mode:** journald (`StandardOutput=journal`) for the daemon log. Audit log (`/var/log/dddns-audit.log`) rotates at 10 MB to `.old`.
- **Cron mode:** `>> /var/log/dddns.log 2>&1` — no rotation.

**Target — cron mode rotation options:**

| Option | Pros | Cons |
|---|---|---|
| A. `logger -t dddns` → journald | Simplest, uses existing OS rotation | journald config on UniFi OS is minimal; may not retain long-term |
| B. Drop `/etc/logrotate.d/dddns` | Standard Linux pattern, handles multi-year | Requires logrotate on UniFi OS; not guaranteed |
| C. Size-rotate in-process | Fully self-contained | Adds code; cron-mode is a one-shot, so rotation lives in the bootscript or a wrapper |

Recommendation: **Option A for cron mode**. journald is already there (systemd is PID 1). It handles rotation via `SystemMaxUse=` in `journald.conf`. Document the default config + how to raise it for users who want years of history.

**Target:**
- Cron entry changes from `*/30 * * * * root /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1` to `*/30 * * * * root /usr/local/bin/dddns update --quiet 2>&1 | logger -t dddns`.
- Document `journalctl -t dddns --since yesterday` in `docs/udm-guide.md`.
- Audit log rotation stays as-is (the 10 MB `.old` swap is already fit for multi-year).

**Risks:** low. Log routing change, no new code.

---

## 7. Upgrade flow

**Current state:** `scripts/install-on-unifi-os.sh` downloads the latest release, verifies SHA-256, and preserves mode across re-runs via boot-script-marker parsing. Re-running the installer upgrades.

**Gaps:**

1. **No in-place upgrade from the binary itself.** `dddns upgrade` doesn't exist. Users must re-run the installer. Acceptable for a minimal CLI, but a `dddns upgrade` subcommand could be a thin wrapper that curls the installer.
2. **No rollback.** The installer downloads, verifies, replaces. If the new binary is broken (panics on start, fails `dddns config check`), the prior version is gone.
3. **No `dddns --version` → "new version available" hint.** Not critical; installer covers this.

**Target:**
- Add `dddns upgrade` as a thin wrapper: `curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash -s -- --upgrade`. Preserves mode automatically via the installer's existing logic.
- Add a backup-and-restore step to the installer: copy `/data/dddns/dddns` to `/data/dddns/dddns.prev` before overwriting; on install failure (e.g., `dddns config check` rejects), roll back.
- Skip the "new version available" feature — over-engineered.

**Risks:** low. Wrapper + backup are additive.

---

## 8. Console testability + token-once security

**Current state:**
- `dddns serve test` — hits loopback `127.0.0.1:53353/nic/update` with Basic Auth, exercises CIDR + auth + handler + Route53 pipeline. The handler UPSERTs the real local WAN IP (ignores `--ip` flag per L4 defense).
- `dddns verify` — reads the Route53 record + cross-checks three public resolvers.
- `dddns update --dry-run` — logs what would happen without touching Route53.
- `dddns config rotate-secret [--init]` — generates a 256-bit secret, prints once, stores encrypted, prompts user to paste into UniFi UI.

**Token-once lifecycle (already fit-for-purpose):**

1. `rotate-secret --init` runs during installer, `crypto/rand` → 64 hex chars.
2. Installer prints the secret ONCE in a framed block with UI-update instructions.
3. Secret is stored at rest in `config.secure` encrypted by the device key.
4. User copies to UniFi Console → Dynamic DNS → Password field.
5. Terminal history retained until the user closes the session. Operator responsibility to `history -c` or close terminal.
6. To rotate later: `dddns config rotate-secret` (same flow without `--init`).

**Gap:** the installer prints the secret in a block that could be trimmed mid-scrollback. Add a visible "write this down NOW — you cannot recover it later" banner.

**Target:**
- Keep `dddns serve test` as the canonical daemon smoke test. Document the three-step console flow in `docs/udm-guide.md`:
  1. `dddns config check` — validates config + probes AWS credentials.
  2. `dddns serve test` — exercises the full serve pipeline end-to-end.
  3. `dddns verify` — confirms Route53 record matches current WAN IP.
- Add a framed "DO NOT LOSE" banner around the printed secret in `rotate-secret`.
- No new command — the existing trio covers the flow.

**Risks:** low. Wording + docs.

---

## Cross-cutting — exit criteria

Release is cut when all of:

- [ ] `go.mod` has two direct deps: `cobra`, `yaml.v3`. Viper and its 14 transitive deps gone.
- [ ] Binary <11 MB on `linux/arm64` with `-s -w -trimpath`.
- [ ] `go test -race ./...` green. Lint gate (no `continue-on-error`).
- [ ] Cron-mode logs route through journald (`logger -t dddns`).
- [ ] Installer runs the 11 environment checks from §5. `--diagnose` exists.
- [ ] One real UDR7 install + one real DNS update verified via `dddns serve test` from the UniFi console terminal. (Blocked on SSH info.)
- [ ] Docs grep clean of `skip_proxy_check`, `IsProxyIP`, `--check-proxy`, `ip-api`, `dddns daemon`.
- [ ] `.goreleaser.yaml` has `-trimpath`. CI and release both produce identical-shape binaries modulo ldflags.
- [ ] `v0.2.0` tag cut; GoReleaser publishes.

## Sequencing

1. ~~Viper removal (item 1) — blocks binary-size check in exit criteria.~~ **Done.**
2. CI workflow tweaks (item 2) — parallel with 1.
3. Memory audit + cron log rotation (items 3, 6) — after 1.
4. Docs refresh (item 4) — parallel with 3.
5. Installer upgrade (items 5, 7, 8 banner) — after 3/6 so new log routing is the default.
6. On-device validation on UDR7 — after 5. Requires user-provided SSH.
7. Tag release — after all above.

## Open questions

- **UDR7 SSH info** — still pending from earlier in this session. Required for item 6 of sequencing.
- **CI size cap 15 MB vs 20 MB** — with viper gone and the SDK gone, current binary is 12.7 MB. Keeping at 15 MB leaves 2.3 MB of headroom. Raising to 20 MB masks future regressions. Recommend: keep at 15 MB; the gate is doing its job.
