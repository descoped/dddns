# Community release plan

**Status:** Planned — not started. Captures gaps identified at the end of v0.2.0-rc.2 validation on UDR7, plus the roadmap hooks for the community announcement.
**Confidence:** High for the pre-release checklist; medium for the multi-provider sequencing (depends on user demand after announcement).
**Last reviewed:** 2026-04-18

## Scope

Everything that should happen between **"v0.2.0 is tagged"** and **"community post on community.ui.com is written"**, plus the forward roadmap the post should *link to* but not promise.

## Out of scope

- The provider interface design — already in `0_provider-architecture.md`.
- Which providers to add and in what order — already in `1_provider-catalog.md`.
- Event-driven IP detection on non-UniFi platforms — see `2_non-unifi-event-detection.md`.
- `dddns install` / `doctor` / `uninstall` zero-question bootstrap — already in `20260417T2033Z_cli-for-idiots.md`.

## What's been validated on UDR7 (v0.2.0-rc.2)

- Install (upgrade path from v0.1.1).
- Rollback (restores prior binary + boot script + cron entry from `.prev`).
- Re-install after rollback.
- `wanip.FromInterface("")` public-IPv4 fallback under policy-based routing.
- `config.secure` cross-version decrypt (v0.1.1 → v0.2.0; same key derivation, same salt).
- Cron-mode `--quiet` silent no-op.
- Dry-run full update chain.
- All three installer safety gates (preflight / snapshot / smoke) with auto-rollback on failure.

## What's NOT validated (gaps by severity)

### High — hits the community on day one

1. **UDM Pro / UDM SE / UDM Pro Max** — the dominant UniFi Dream devices. UDR7 is the new kid. Most issue reports will come from UDM-family users. The wanip code path differs: UDM takes the `/proc/net/route` default-route path, UDR takes the fallback. Both are covered by tests, but not by device-in-hand verification.
2. **Serve mode on UniFi Dream** — validated as far as it can be: `dddns config set-mode serve` produces a working systemd unit, `dddns serve test` + `curl` from the shell succeed end-to-end including Route53 UPSERT, audit log, status file, GOMEMLIMIT ceiling. **But the UniFi UI → inadyn → dddns path doesn't work**: UniFi's `inadyn` is invoked with `-b eth4`, and the kernel's `SO_BINDTODEVICE=eth4` constraint forces `connect(127.0.0.1)` through UniFi's WAN policy table (`201.eth4`), out the wire, dropped. `ip route get 127.0.0.1 oif eth4` shows the failure mode deterministically. Documented as "experimental on UniFi Dream" in `docs/udm-guide.md`. Community ideas welcome. Serve mode on non-UniFi platforms (Pi, Linux servers, Docker) is unaffected and fully working.
3. **Fresh install** — all RC testing was upgrade. The fresh-install branch exercises: interactive mode prompt, default `config.yaml` creation, `chmod 700/600` on fresh directories, the "proceed?" TTY prompt.

### Medium — will bite a small fraction of users

4. **Uninstall** — coded, never run. `install → uninstall → probe` cycle has zero verification.
5. **UniFi OS 2.x / older firmware** — systemd 232-ish vs the 247 on UDR7. Bash 4.x vs 5.x. `/etc/init.d/cron` layout may differ. Hard to pre-test without the hardware; the probe surfaces firmware/systemd/bash versions so remote diagnosis is tractable.
6. **`ip_source: remote` explicit** — older cron-mode installs may have this set. Code supports it but the combined "remote IP source + upgrade path" hasn't been exercised together.

### Low — edge cases

7. **Non-UDM platforms** (Pi, macOS, Windows, Docker) — dddns code supports them (per CLAUDE.md) but the UniFi installer doesn't. Not a UniFi-community concern for this announcement.
8. **IPv6** — dddns is v4-only by design. Users on v6-heavy networks may see unexpected behavior if they don't have a v4 WAN.

## Pre-announcement checklist

Order matters — items build on each other.

1. ~~Serve-mode smoke~~ — **done**. `dddns serve test` + raw curl reach the listener cleanly; full round-trip Route53 UPSERT validated (bogus write + correction). UniFi-UI → inadyn → dddns path does NOT work; diagnosis documented, flagged as experimental on UniFi Dream. Cron mode is the UniFi path.
2. **Uninstall + fresh-install cycle** — on UDR7 itself: `--uninstall`, `rm -rf /data/.dddns /var/log/dddns.log`, then run the installer fresh. Confirm interactive mode prompt, default config creation, `dddns config check` reports "YOUR_ACCESS_KEY" placeholder cleanly. 15 min. **Optional — not blocking v0.2.0** given the extensive upgrade/rollback/re-install testing already done.
3. **Tag v0.2.0** (no code changes from rc.3 binary — only commit SHA + build date differ).
4. **Community post** framing cron as the UniFi path, serve as the same-host-client path (non-UniFi).

## Probe upgrades before announcement (v0.2.1 patch or fold into v0.2.0)

The probe covers ~70% of remote diagnosis today. Two additions close the most common support gaps:

### Probe-1. Upstream reachability checks

Add a `[connectivity]` section. Each target checked via `curl -fsI --max-time 5`; reports OK / fail / timeout. Zero privacy cost — no IPs, no hostnames leaked.

Targets:
- `https://github.com/descoped/dddns/releases` — can the device reach the release server?
- `https://checkip.amazonaws.com/` — baseline Route53-related endpoint
- `https://route53.amazonaws.com/` — actual Route53 API endpoint (HEAD only, no auth)
- `https://api.github.com/repos/descoped/dddns` — GitHub API (for `--version` tag resolution)

Output shape:
```
[connectivity]
  GitHub releases:  OK (302 redirect)
  checkip.amazonaws.com: OK (200)
  Route53 API:      OK (403 — expected without auth)
  GitHub API:       OK (200)
```

A 403 on Route53 is *success* for reachability — it means the TCP handshake + TLS + HTTP round-trip all worked. The probe should classify any 2xx/3xx/403 as OK and anything else as fail.

### Probe-2. Firmware version

Add to `[system]`:
```
  firmware:        4.2.8 (UniFi OS)
```

Sources (in priority order):
1. `/usr/lib/version` (UniFi OS canonical)
2. `/etc/unifi-os/unifi-os.conf` (older firmware)
3. `/etc/motd` banner parse (last-resort)

If none match, show `(unknown)`. Users don't self-report firmware accurately; probe-side extraction is worth 30 lines of shell.

### Probe-3. Wanip interface choice (needs installed binary)

When `${INSTALL_DIR}/${BINARY_NAME}` is present, optionally run:

```bash
${INSTALL_DIR}/${BINARY_NAME} update --dry-run 2>&1 | awk '/Current public IP/ {print; exit}'
```

…but strip the IP and print only the interface name if the binary exposes it. Today it doesn't — would need a `--verbose` flag on `dddns update` that emits `chose iface=eth8` without the IP. **Skip for now; add if support volume justifies.**

## Community support workflow

### GitHub issue template (already exists)

`.github/ISSUE_TEMPLATE/environment-report.md` was created earlier in this session. Update it to:

1. Tell users to paste `--probe` output verbatim (privacy-safe by design — nothing to redact).
2. Add a "what did you try?" box.
3. Link to `docs/unifi-installer.md` (create if missing — today the install instructions are scattered across `README.md` + goreleaser release footer).

### What to explicitly NOT promise in the community post

- "Works on all UDM variants" — say *"tested on UDR7; UDM Pro / UDM SE reports welcome"*.
- "Backwards compatible with all existing dddns installs" — true for v0.1.1 `config.secure` (proven). Older `config.yaml` formats not tested.
- "Supports provider X" — Route53 only, with multi-provider on the roadmap. Don't hint at a timeline.

### Suggested community-post framing

> **First stable release of dddns with serve mode.** Tested personally on UDR7 cron + serve. If you're on UDM Pro / UDM SE / other UDM variant and hit something, run `bash <(curl -fsL .../install-on-unifi-os.sh) --probe` and paste the output at github.com/descoped/dddns/issues — the probe is privacy-safe by design (no IPs, no config values, no log contents).

## Forward roadmap (link from the post, don't promise)

### v0.3.0 priorities

From `20260417T2033Z_cli-for-idiots.md`:
- `dddns install` / `doctor` / `uninstall` zero-question subcommands.
- Shell installer shrinks to ~40 lines (the heavy lifting moves into the binary).
- Platform-auto detection for non-UniFi targets (Raspberry Pi, generic Linux, macOS, Docker).

Residual items (originally from the retired `20260417T2002Z_release-prep.md`):
- CI workflow polish (lint-gate, concurrency block, `-trimpath`).
- Cron-mode log routing through journald instead of flat files (`\| logger -t dddns`). Also closes the "no rotation on `/var/log/dddns.log`" gap — journald rotates itself.
- Memory-leak audit for the serve-mode listener under sustained load. Target: RSS stays <20 MB after 24 h of simulated `inadyn` push traffic. Tools: `go test -run TestServer_Integration -race -count=1000` + `pprof -alloc_space` against a sustained-load harness.
- Docs refresh — **done 2026-04-18** alongside this plan.

Logging UX (identified during v0.2.0 release review):
- `--verbose` flag on `dddns update` as the counterpart to `--quiet`. Today operators wanting "tell me what you're doing" have to use `--dry-run`, which is semantically different (doesn't actually update). Should emit the `logInfo` closure output unconditionally.
- Add a "Where do my logs live?" section to `docs/troubleshooting.md` documenting the four log sinks (cron operational log, serve audit JSONL, serve status snapshot, systemd journal) and when to look at each. Issue-triage value: users reporting "serve mode isn't working" often only check `/var/log/dddns.log`, miss `serve-audit.log` + `journalctl -u dddns`.

### Multi-provider rollout order

The `1_provider-catalog.md` tiers are the authoritative list. Pragmatic ordering once the provider framework (per `0_provider-architecture.md`) lands:

**Wave 1** (cover the majority of UniFi-community domain registrars):
- **Cloudflare** — free tier, JSON API, scoped tokens. Expect this to be the #2 request after Route53.
- **DuckDNS** — free, simple `GET ?ip=...&token=...`. Lowest implementation cost (~60 LoC), highest hobbyist demand.

**Wave 2** (common DNS hosts):
- **Namecheap** — XML API, IP allowlist requirement is annoying but doable.
- **GoDaddy** — JSON, widespread registrar.
- **Gandi LiveDNS** — JSON, clean API.

**Wave 3** (developer-focused):
- **DigitalOcean** — JSON, Bearer token.
- **Hetzner DNS** — JSON, API token.
- **Linode** — JSON, Bearer.

**Wave 4** (niche / regional):
- **Porkbun**, **Domeneshop**, **Name.com** — pull-request-driven; only land with external contributor interest.

**Explicitly deferred:** RFC 2136 `nsupdate` (BIND), Hurricane Electric, DynDNS-protocol mirrors (inadyn already fronts these via serve mode). Track in `ai_docs/1_provider-catalog.md` Tier-3 if requested.

### Platform expansion (Phase 2 of cli-for-idiots)

- **Raspberry Pi** — per `6_raspberry-pi-support.md` (if it exists; otherwise fold into cli-for-idiots).
- **macOS launchd** — already spec'd in `cli-for-idiots.md`.
- **Windows schtasks** — scheduler dispatch.
- **Docker** — sidecar pattern; no installer, just documentation + compose snippet.

## Acceptance for this plan

This plan is done when:

1. The three pre-announcement checklist items (serve-mode smoke, uninstall/fresh-install cycle, probe upgrades) are shipped.
2. `v0.2.0` is tagged.
3. `.github/ISSUE_TEMPLATE/environment-report.md` is updated to reference `--probe` output.
4. `docs/unifi-installer.md` (or equivalent) exists as a stable page to link from the community post.

Each of the four items is independently trackable; nothing blocks tagging v0.2.0 except serve-mode smoke + fresh-install cycle.
