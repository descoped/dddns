# Session State — UniFi DDNS Bridge Implementation

Snapshot of where the `docs/unifi-ddns-bridge` branch stands at the close of the implementation session. Intended to let a future session pick up without re-deriving context from commits alone.

## Branch / HEAD

- **Working branch:** `docs/unifi-ddns-bridge` (branched off `feature/providers` on 2026-04-17).
- **HEAD:** `7655b80` — *refactor: supervise serve mode with systemd instead of a shell while-loop*.
- **Tests at HEAD:** 206 Go tests across 14 packages, passing under `go test -race`. `shellcheck` clean on `scripts/install-on-unifi-os.sh`. `bash -n` clean.
- **Not yet merged.** The plan is to merge back into `feature/providers` once on-device validation is done (see "Pending" below).

## What's Done — Phase Summary

All steps from `ai_docs/8_unifi-ddns-bridge.md` §14 Implementation Plan are complete, plus a post-F systemd refactor the plan didn't originally include.

### Phase 0 — Pre-existing bug fixes (orthogonal to UniFi feature)

| Step | Commit | What |
|------|--------|------|
| 0.1 | `99184e9` | `filepath.Dir` replaces ad-hoc `"/"` slicing in `writeCachedIP` + `CreateDefault` |
| 0.2 | `294a5c2` | `IsProxyIP` surfaces API failures instead of collapsing them to "not a proxy" |
| G6  | `d3a521f` | Retired `ip-api.com` entirely — added `myip.ValidatePublicIP` via stdlib (`IsGlobalUnicast`, `IsPrivate`, `To4`). Removed `IsProxyIP`, `geoLocation`, `SkipProxy` config field, `--check-proxy` flag. Net −212 lines. |
| 0.3 | `71bc8ec` | `cmd/root.go:initConfig` now `os.Exit(1)`s on non-not-found YAML parse errors |
| 0.4 | `0467f50` | `route53.go` `strings.HasSuffix` guard replaces the empty-hostname-panicking `fqdn[len-1]` |
| *(Phase 0.5 removed)* | — | Code already soft-fails on `IsProxyIP` errors; no change needed. Discovered during implementation review. |

### Phase A — Prep refactors (no behavior change)

| Step | Commit | What |
|------|--------|------|
| A1 | `9a6b8b0` | Extracted `internal/updater` with `Update(ctx, cfg, Options) → *Result`. Plumbed `context.Context` through `internal/dns/route53.go` (all `context.TODO()` gone from the request path). Added signal handling (`signal.NotifyContext SIGINT/SIGTERM`) in `cmd/update.go`. Cache helpers moved from `cmd/` to `internal/updater/`. New `DNSClient` interface for test injection. |
| A2 | `03a6b14` | Factored `crypto.EncryptString` / `DecryptString` out of `EncryptCredentials`. Added table-driven round-trip + GCM tamper-detection tests to the existing `device_crypto_test.go`. |

### Phase B — Config schema

| Step | Commit | What |
|------|--------|------|
| B1 | `a7ae93e` | New `ServerConfig` struct (Bind, SharedSecret, AllowedCIDRs, AuditLog, OnAuthFailure, WANInterface) with dual `mapstructure`/`yaml` tags. New top-level `IPSource` on `Config`. `ServerConfig.Validate()` fails closed on empty `allowed_cidrs`, malformed bind, missing secret, unparseable CIDRs. |
| B2 | `ef38e1b` | New `SecureServerConfig` mirror type with `SecretVault` in place of `SharedSecret`. `SaveSecure` encrypts on write, `LoadSecure` decrypts on read. Added a tamper-detection test. |

### Phase C — Server core

| Step | Commit | What |
|------|--------|------|
| C1 | `58f7f3c` | `internal/server/cidr.go` — `IsAllowed(remoteAddr, cidrs)` with IPv4/IPv6 + zone support, fail-closed on empty list. 18 subtests. |
| C2 | `d0e92a0` | `internal/server/auth.go` — `Authenticator` with `crypto/subtle.ConstantTimeCompare` + sliding-window lockout (5 fails / 60 s → 5 min block). Three-valued `AuthResult` enum. Injectable clock. Race-clean under 2000-call concurrent exerciser. |
| C3 | `fd5d731` | `internal/server/audit.go` — JSONL `AuditLog` with mutex-guarded append and 10 MB rotation to `.old`. `AuditEntry` fields: ts, remote, hostname, myip_claimed, myip_verified, auth, action, route53_change_id, error. |
| C4 | `5504251` | `internal/wanip` — `FromInterface(name)` reads the WAN interface directly; auto-detect via `/proc/net/route`; rejects loopback, link-local, multicast, RFC1918, CGNAT (`100.64/10`), IPv6. Plus `resolveIP(cfg)` dispatcher in `internal/updater` with `local`/`remote`/`auto` handling (UDM profile → local, else remote). |
| C5 | `f9a1e12` | `internal/server/handler.go` + `status.go` (writer). Implements `http.Handler` for `/nic/update` — CIDR → method → auth → hostname → local-WAN-IP → updater → dyndns response mapping + audit + status refresh. 30 s timeout around the updater call. Always uses local IP for the UPSERT; `myip` query param is only captured in the audit entry. |
| C6 | `271bdde` | `internal/server/server.go` + `cmd/serve.go`. Wires the handler chain from validated config, graceful shutdown via `http.Server.Shutdown(5s)`. Fail-closed on config validation. `Server.binder` hook for deterministic ephemeral-port tests. |

### Phase D — Operational commands

| Step | Commit | What |
|------|--------|------|
| D1 | `d211070` | `dddns serve status` + `server.ReadStatus` + exported `StatusPath`/`AuditPath`. Fixed a latent `-race` flake in `TestStatusWriter_Atomic`. |
| D2 | `d4b6827` | `dddns serve test` with `loopbackURL` helper that translates `0.0.0.0`/`::` to `127.0.0.1`. Exit 0 on `good`/`nochg`. |
| D3 | `c3a4363` | `dddns config rotate-secret [--init] [--quiet]`. Plaintext vs encrypted detected from path suffix. `--init` creates fail-closed default server block. Fixed `SaveSecure` to chmod 0600 before rewriting its own 0400 file. |
| D4 | `58f813f` | `internal/bootscript` (pure `Generate(Params)`) + `dddns config set-mode {cron\|serve}`. No auto-exec — writes the boot script and prints apply-or-reboot instructions. |

### Phase E — Installer integration

| Step | Commit | What |
|------|--------|------|
| E1 | `9dcbbd7` | Rewrote `scripts/install-on-unifi-os.sh` around the new subcommands. `--mode cron\|serve` + interactive prompt + upgrade-preserves-mode via boot-script marker parsing. SHA-256 verification against release `checksums.txt`. Serve-mode install prints the UniFi UI values to paste. Uninstall `pkill`s any running serve loop (later updated — see post-F refactor). |

### Phase F — Documentation

| Step | Commit | What |
|------|--------|------|
| — | `75c7d65` | Renamed all `docs/*.md` from `SCREAMING_SNAKE_CASE` to lowercase kebab-case per user preference (memory: `feedback_naming.md`). Updated all cross-references. |
| F1 | `6d84ad2` | `docs/aws-setup.md` IAM policy scoped via `route53:ChangeResourceRecordSetsNormalizedRecordNames` / `RecordTypes` / `Actions` condition keys. Blast-radius explainer. |
| F2 | `a78b3a7` | `docs/udm-guide.md` — Run Modes table + Serve Mode section + Switching Modes section + Monitoring rewritten around a four-row log table + Stale cleanups. |
| F3 | `5ed0518` | `docs/troubleshooting.md` — Serve Mode Issues section with per-dyndns-code subsections (`badauth`, `notfqdn`, `nohost`, `dnserr`, `911`, HTTP 403, lockout). Quick Diagnostics grouped by mode. |

### Post-F: systemd pivot

| Step | Commit | What |
|------|--------|------|
| systemd | `7655b80` | Switched serve-mode supervision from a shell `while true; do dddns serve; sleep 5; done` loop to a systemd unit (`dddns.service`, `Restart=always`, journald, `ProtectSystem=strict`, `ReadWritePaths=/data/.dddns /var/log`). The `/data/` → `/etc/systemd/system/` re-install-from-`on_boot.d`-on-every-boot pattern handles firmware-upgrade persistence. Cron mode kept on `/etc/cron.d/dddns` — no churn for no gain. All docs + installer uninstall + bootscript tests updated. |

## Test / Tooling Status at HEAD

```
go test -race ./...    → 206 passed in 14 packages
go build ./...         → ok
bash -n scripts/…      → ok
shellcheck scripts/…   → silent (no warnings)
go run ./main.go --help → shows: update, serve, config, ip, verify, secure
```

## Pending Work

**Before merging to `feature/providers`:**

- [ ] On-device validation on a real UDR/UDM. The installer, boot script, systemd unit wiring, and the UniFi UI `inadyn` integration have all been exercised in unit tests and with dry-run invocations, but not against a live device. Minimum check-out:
  - `install-on-unifi-os.sh --mode serve` completes end-to-end, prints UniFi UI values, SHA-256 verification passes on the real release artefact.
  - `systemctl status dddns` reports `active (running)` after the boot script runs.
  - UniFi UI `inadyn` sends a request; `dddns serve status` shows the last request; Route53 record updates.
  - `dddns config set-mode cron` swaps cleanly; `/etc/cron.d/dddns` appears; service is stopped + disabled + removed.
  - `dddns config rotate-secret` rewrites the secret; UniFi UI fails with `badauth` until the password field is updated, then succeeds.

- [ ] Release engineering question for the user: this branch ends at `7655b80`. The `feature/providers` branch (parent) also has the doc series (`0_*` through `7_*`). After merge, a release tag would carry both the new feature and the series.

**Explicitly deferred (Phase G in `8_unifi-ddns-bridge.md`):**

- G1 — Multi-hostname support (waits for the v2 multi-target config work in `ai_docs/0_`/`1_`).
- G2 — IPv6 / AAAA record support.
- G3 — Non-root execution via dedicated `dddns` system user.
- G4 — CloudTrail / SNS detection guide.
- G5 — Pluggable notification backend for `on_auth_failure`.
- G6 — *(already completed — ip-api.com retired in commit `d3a521f` during Phase 0)*

## Open Decisions

None blocking. The systemd pivot resolved the last architectural question. If the user wants to:

- **Switch cron mode to a systemd timer** — the research in this session confirms it works on UDM/UDR. The refactor would follow the same pattern as the serve-mode unit (unit + timer lives on `/data/`, copied into `/etc/systemd/system/` from the boot script). Not recommended as a default (no functional gain, more churn, the whole UDM ecosystem uses `/etc/cron.d/`) but a reasonable opt-in.
- **Drop CGNAT rejection from `wanip`** — currently `100.64.0.0/10` is rejected as "non-usable". If a user on CGNAT actually wants to record that address in Route53 for tracking purposes, this would need to be relaxed or made configurable.
- **Widen LAN reachability by default** — currently `allowed_cidrs: ["127.0.0.0/8"]`. If users complain about not being able to test from LAN, the installer could prompt for the home subnet.

## Key Artifacts

- **Design doc:** `ai_docs/8_unifi-ddns-bridge.md` — full spec, security model, implementation plan with per-step completion notes.
- **This doc:** `ai_docs/9_session-state.md` — snapshot for session handoff.
- **Memory (external):**
  - `project_unifi_bridge_branch.md` — this branch's lineage.
  - `project_unifi_systemd.md` — UDM/UDR runs systemd; prefer units over shell supervisors.
  - `feedback_naming.md` — lowercase kebab-case for docs, not SCREAMING_SNAKE_CASE.

## Full Commit Log (on this branch, off `main`)

Reverse-chronological. 35 commits total (including the parent provider-series commits from `feature/providers`).

```
7655b80 refactor: supervise serve mode with systemd instead of a shell while-loop
5ed0518 docs: add serve-mode diagnostics to the troubleshooting guide
a78b3a7 docs: add serve-mode coverage to the UDM guide
6d84ad2 docs: tighten aws-setup IAM policy to record-scoped UPSERT
75c7d65 refactor: rename docs to lowercase kebab-case
9dcbbd7 feat: rewrite UniFi installer around set-mode and add SHA-256 verification
58f813f feat: add bootscript generator and dddns config set-mode
c3a4363 feat: add dddns config rotate-secret for serve-mode credential rotation
d4b6827 feat: add dddns serve test subcommand for SSH-side debugging
d211070 feat: add dddns serve status subcommand and status-file reader
271bdde feat: add serve-mode HTTP Server lifecycle and dddns serve command
f9a1e12 feat: add serve-mode HTTP handler and status-file writer
5504251 feat: add internal/wanip and wire cron-path IP resolution to ip_source
fd5d731 feat: add JSONL audit log with size-based rotation
d0e92a0 feat: add Authenticator with constant-time compare and sliding-window lockout
58f7f3c feat: add CIDR allowlist helper for the serve-mode handler
ef38e1b feat: encrypt server shared secret at rest in the secure config
a7ae93e feat: add ServerConfig struct and IPSource field to Config
03a6b14 refactor: factor EncryptString/DecryptString out of EncryptCredentials
9a6b8b0 refactor: extract update flow into internal/updater with bounded context
0467f50 fix: guard against empty hostname in Route53 client
71bc8ec fix: exit non-zero when the config file has a YAML parse error
d3a521f refactor: retire ip-api.com proxy check, replace with stdlib IP validation
294a5c2 fix: surface ip-api.com failures instead of silently reporting "not a proxy"
87d8d4a docs: mark path fix complete with status and findings
99184e9 fix: use filepath.Dir for cache and config directory paths
8f6e63f docs: tighten updater dispatch wording and C4 scope
5dc02e8 docs: remove Phase 0.5 and tighten ownership of ambiguous steps
83467aa docs: switch IP verification to local WAN interface over checkip
da15a3a docs: add Phase 0 bug fixes and harden A1/E1 in implementation plan
6ba2d77 docs: harden UniFi DDNS bridge spec with layered security model
a5f8432 docs: rewrite UniFi DDNS bridge spec as exclusive-mode design
3d69786 docs: add UniFi-to-Route53 DDNS bridge spec with assessment
```

(Earlier commits `c8fc75e`, `5827373`, `d7cd74c`, `6c687cb`, `9dd4d7b`, `5cdd46c`, `55c6f13` are the pre-existing provider-series docs from `feature/providers` — unchanged by this branch.)

## How to Resume

1. `git checkout docs/unifi-ddns-bridge` (already up-to-date on `origin`).
2. Read `ai_docs/8_unifi-ddns-bridge.md` for the design.
3. Read this file for the status.
4. Do the on-device validation checklist under "Pending Work".
5. When satisfied: `git checkout feature/providers && git merge --no-ff docs/unifi-ddns-bridge`.
