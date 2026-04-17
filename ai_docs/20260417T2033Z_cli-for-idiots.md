# dddns install — design for a CLI-for-idiots

**Status:** Planned — design awaiting user approval before implementation.
**Confidence:** High — built from the shipped command surface and profile detector. No speculative dependencies.
**Last reviewed:** 2026-04-17

## Scope

One command that bootstraps dddns on whatever box it lands on. Succeeds without the user knowing what cron is, what systemd is, what a boot script is, or what a shared secret is. The expert escape hatches — every existing subcommand, every existing flag — stay intact. No options are removed.

## Out of scope

- Platforms with no scheduler (bare kernel, embedded). Fail loud.
- Provider abstraction (see `0_provider-architecture.md`). This is Route53-only until multi-provider lands.
- Editing config post-install through `install`. For that, use `dddns config init` (kept).

## Personas and desired outcomes

| # | Persona | Starting point | Single-command outcome |
|---|---|---|---|
| 1 | Pure idiot | Downloaded binary, no AWS yet | `dddns install` prompts for access key, secret, hostname, zone. Installs. Runs. |
| 2 | Prepared idiot | Has AWS creds in env | `dddns install` reads env, zero prompts, installs and runs. |
| 3 | Scripted expert | Automating via Ansible/Pulumi | `dddns install --hostname X --zone Z --access-key ... --secret-key ... --non-interactive` |
| 4 | Upgrader | Existing install, new release | `dddns install --upgrade` preserves mode, replaces binary, re-applies boot script. |
| 5 | Recovery operator | Something broken | `dddns doctor` tells them what's wrong with one glance. |

Persona 1 is the north star. Everyone else benefits from the same code path with non-interactive flags.

## Platform detection — decision table

Performed once at the start of `install`. Cached on the returned `profile.Profile` so subsequent phases don't re-probe.

| Probe | Result | Classification |
|---|---|---|
| `/proc/ubnthal/system.info` exists | yes | `unifi` (read model; use `/data/.dddns`, `/data/on_boot.d`) |
| `/etc/rpi-issue` exists OR `/proc/device-tree/model` contains "Raspberry Pi" | yes | `raspberry-pi` (`~/.dddns`, systemd timer) |
| `/.dockerenv` OR `/run/.containerenv` | yes | `docker` (`/config/`, no persistence — container runtime handles it) |
| `uname -s` == Darwin | yes | `macos` (`~/.dddns`, launchd — or cron if user prefers) |
| `uname -s` == Linux AND `test -d /run/systemd/system` | yes | `linux-systemd` (`~/.dddns` or `/etc/dddns`, systemd timer) |
| `uname -s` == Linux AND crontab writable | yes | `linux-cron` (`~/.dddns`, crontab entry) |
| else | — | `unknown` — print "no scheduler available, run `dddns update` manually" and exit 0. Config still written. |

Most of this already exists in `internal/profile/profile.go`. The missing piece is `raspberry-pi`, `linux-systemd`, `linux-cron` refinement — currently `Linux` is one bucket. Keep the existing profile for paths; add a separate `schedulerProbe()` helper that returns `{systemd, cron, launchd, none}` for mode selection.

## Mode picker — the one opinionated choice

```
if platform == unifi:        mode = serve   (L4-defended inadyn push, shipped)
elif platform == docker:     mode = foreground (run in PID 1, container orchestrator cycles us)
elif scheduler == systemd:   mode = timer   (systemd .timer unit firing dddns update)
elif scheduler == cron:      mode = cron    (crontab or /etc/cron.d entry)
elif scheduler == launchd:   mode = launchd (macOS LaunchAgent)
else:                        mode = manual  (print next-step for the user)
```

The user can override every one of these with `--mode {serve|timer|cron|launchd|foreground|manual}`. The auto-picker only runs when `--mode` is absent.

`serve` stays UniFi-only. On a Pi there's no `inadyn` to push to it — serve mode would bind but nobody would call it.

## `dddns install` — syntax

```
Usage:
  dddns install [flags]

Flags (new, small set):
      --hostname string      Route53 FQDN to keep updated (or $DDDNS_HOSTNAME)
      --zone string          Hosted zone ID (or $DDDNS_ZONE_ID)
      --access-key string    AWS access key (or $AWS_ACCESS_KEY_ID)
      --secret-key string    AWS secret key (or $AWS_SECRET_ACCESS_KEY)
      --ttl int              Record TTL in seconds (default 300)
      --mode string          Override the auto-picked mode (see picker above)
      --upgrade              Re-apply boot artefact after replacing the binary
      --non-interactive      Fail if any input would prompt
      --reconfigure          Treat an existing config as absent; prompt again

Inherited global flags (unchanged):
      --config string        Config file location

Inherited existing subcommands (unchanged):
  dddns update / verify / ip
  dddns config {init,check,set-mode,rotate-secret}
  dddns serve / serve status / serve test
  dddns secure {enable,test}
```

### Input priority

Flags > env vars > existing valid config > interactive prompts > fail.

Example: `dddns install` on a box with `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` set and a half-written `~/.dddns/config.yaml` (missing hostname):
- Access key → from env (idiot-friendly, standard AWS convention)
- Secret key → from env
- Zone → read from existing config
- Hostname → **prompt** (missing everywhere)
- TTL → 300 default
- Mode → auto-picker

If `--non-interactive` set, the missing hostname fails with a specific message rather than hanging on a prompt.

## `dddns doctor` — diagnostic for persona 5

Runs a fixed sequence of checks and prints `✓` / `✗` per line, with a one-sentence fix on each failure. No prompts, no mutations. Exit code 0 only if every gate passes.

| Check | Fix on failure |
|---|---|
| Binary in PATH and executable | `sudo dddns install` |
| Config file present (`config.yaml` or `config.secure`) | `dddns install` |
| Config parses as YAML | Re-run `dddns install --reconfigure` |
| Required fields present (aws creds, zone, hostname, ttl) | `dddns config init` |
| AWS credentials valid (probe `ListResourceRecordSets`) | Update `access_key` / `secret_key`; re-run |
| Hosted zone exists and hostname suffix matches | Check zone ID and hostname in `config.yaml` |
| WAN IP resolvable and publicly routable | `dddns ip` to debug |
| For `serve` mode: unit active; port 53353 bound | `systemctl status dddns.service` |
| For `cron` mode: cron entry present; last run ≤ interval+5min | Check `/etc/cron.d/dddns`, `journalctl -t dddns` |
| Last audit entry timestamp within 24h (serve only) | Check inadyn config in UniFi UI |

## Install flow — step by step

```
1. Platform probe           → profile (UDM / Pi / Linux / macOS / Docker / unknown)
2. Existing install scan    → mode, prior config, binary path (for upgrade)
3. Input assembly           → flags → env → existing config → prompt
4. Config validate + persist → Config.Validate, SavePlaintext or SaveSecure
5. Route53 probe            → list zone once; assert hostname-in-zone
6. Mode selection           → picker (or --mode override); auto-sanity if "serve" and not UniFi
7. Mode install             → write boot script / systemd unit / cron entry / launchd plist
8. Activate                 → systemctl enable+start, cron restart, launchctl load
9. Post-install verify      → doctor checks for the selected mode (subset of full doctor)
10. Summary                 → one screen of "where things are", next-step hints, token for serve
```

Each step is a named function in a new `internal/install` package. Single-file is fine until it exceeds 400 lines.

## Edge case catalog

For each: what happens today (if known), what should happen, what the user sees.

| # | Condition | Current (shell) | `dddns install` behaviour | User sees |
|---|---|---|---|---|
| 1 | Not root, persistent install | `check_root` exits | Same — `os.Geteuid() != 0` ⇒ exit with `run with sudo` | `error: persistent install requires root — re-run with sudo` |
| 2 | UniFi but `/data` read-only | shell would fail on `mkdir` | Probe `/data` writability early; exit with `/data is read-only — check filesystem` | clear pre-flight error |
| 3 | UniFi but `udm-boot.service` missing | shell prompts to install `unifios-utilities` | `install` detects absence and either (a) bootstraps `unifios-utilities` non-interactively, or (b) bails with the one-line fix command | no prompt |
| 4 | Multiple WAN interfaces | `wanip` picks default-route iface (shipped) | Unchanged. `--wan-interface` flag already exists in `ServerConfig` if needed | — |
| 5 | Existing config with invalid YAML | current `dddns config check` reports it | `install` without `--reconfigure` respects the file — prints "existing config is broken, re-run with --reconfigure to overwrite" | clear recovery path |
| 6 | AWS creds supplied but wrong | fails later at cron time | Step 5 (Route53 probe) catches it at install time | `AWS credential check failed: InvalidClientTokenId — recheck access_key` |
| 7 | Hostname not in the specified zone | would 404 on the first update | Step 5 also checks hostname suffix against zone | `hostname "home.example.com" does not sit under zone "example.net" — pick one` |
| 8 | GitHub unreachable (air-gapped install) | `get_latest_version` curl fails | `install` itself doesn't hit GitHub — it's the already-extracted binary doing the work. Shell wrapper's job to fail on curl. | shell wrapper prints "cannot reach GitHub — download release tarball manually" |
| 9 | IPv6-only WAN | `wanip` rejects (shipped) | Unchanged. `install` inherits the same rejection and emits a clear message | `no public IPv4 on WAN interface — dddns manages A records only` |
| 10 | CGNAT / RFC1918 WAN | `wanip` rejects (shipped) | Same | `WAN interface has non-public IP — dddns cannot manage a public DNS record for you` |
| 11 | Re-run `install` on identical config | — | Idempotent — profile + mode same ⇒ no-op with "nothing to do" banner | `Already installed, mode=serve, last update 6 min ago. Nothing to change.` |
| 12 | Mode switch (`--mode cron` on a serve install) | `dddns config set-mode` already exists | `install --mode cron` wraps `set-mode` and re-runs the boot script. Idempotent. | `Switched from serve to cron. Cron entry: /etc/cron.d/dddns` |
| 13 | Two users' configs collide (config in `/etc` and `~`) | — | `install` respects `--config`; default honours `profile.Detect()`. Warn if both exist. | warning + pick explicit path |
| 14 | Upgrade path, binary already newest | shell `install_binary` short-circuits | `install --upgrade` detects same version, re-applies boot artefacts only | `Already on v0.2.0. Re-applied boot script. No restart needed.` |
| 15 | UniFi firmware wipe (`/etc/systemd/system/` gone, `/data` intact) | on-boot script re-installs unit | Unchanged — this is the canonical `/data/on_boot.d/` pattern | daemon back within ~30s of boot |
| 16 | User runs `dddns install` on macOS without `--mode`, auto-picks launchd | — | Auto-picks `launchd`, writes `~/Library/LaunchAgents/io.descoped.dddns.plist`, `launchctl load` | `Installed launchd agent. Runs every 30 min.` |
| 17 | User runs `dddns install` in Docker | — | Auto-picks `foreground`. Writes config, prints "exec: dddns update --quiet in your CMD" | hint for `Dockerfile` wiring |
| 18 | `--non-interactive` with missing input | — | Fails with the specific missing field in the error, not a generic prompt | `missing --hostname (or DDDNS_HOSTNAME) — required for non-interactive install` |
| 19 | Prompt on a non-tty (CI pipe) | shell installer stumbles | `install` detects `isatty(stdin) == false` and treats as `--non-interactive` implicitly | same specific missing-field error |
| 20 | Disk full on /data | shell fails on write | `install` probes `/data` free space early | `Low disk space on /data: 12 MB free (>= 50 MB required) — free space and retry` |
| 21 | Clock skew > 15 min | Route53 probe returns `SignatureDoesNotMatch` | Catch the specific error shape and print | `System clock is too far off — check NTP (SigV4 rejects requests > 15 min skewed)` |
| 22 | Uninstall leaves config | shell `--uninstall` already does | `install --uninstall` (new subcommand, thin wrapper) does the same. `--purge` removes config. | `Uninstalled. Config preserved at /data/.dddns (use --purge to remove).` |
| 23 | Install over `.secure` (encrypted) config | `config.Load` decrypts | `install` detects `.secure`, keeps encryption, rotates secret on upgrade only when `--rotate-secret` passed | never silently re-plaintext a `.secure` file |
| 24 | Shared secret rotation during upgrade | — | `install --upgrade` preserves secret by default. `install --rotate-secret` regenerates (prints once, user updates UniFi UI) | clear banner before + after |
| 25 | User pastes their AWS creds into an ENV file that persists | outside dddns's control | Docs say "prefer `dddns secure enable` over env files"; no enforcement | documentation only |
| 26 | Cron mode on systemd-less Linux (busybox) | — | Picker falls through to `cron` if `/run/systemd/system` absent | `Installed crontab entry — dddns runs every 30 min` |
| 27 | Two install scripts racing | shell has no lock | `install` acquires `/var/lock/dddns.lock` (`flock`) | `Another install is in progress — wait or stat /var/lock/dddns.lock` |
| 28 | Install with `--config /tmp/test.yaml` (non-root writable) | — | Honours the flag. Uses user-level scheduler (cron user, not `/etc/cron.d`). Explicitly labels as "user install" | `User-mode install. Config at /tmp/test.yaml. Cron entry in $USER crontab.` |

## Token lifecycle — preserved verbatim

The UniFi bridge's token-once flow is untouched:

1. `install` on UniFi → auto-runs `rotate-secret --init --quiet` (existing). Secret is generated with `crypto/rand`, written encrypted to `.secure` if encryption is enabled.
2. Secret is printed ONCE in a framed block with the UniFi UI paste values (Service=Custom / Hostname / Username / Password / Server). This is the only opportunity to see it.
3. User copies into Settings → Internet → Dynamic DNS → Create. Terminal window can be closed and discarded.
4. To rotate: `dddns config rotate-secret` (existing, kept). Prints once. Update UniFi UI.
5. `install --upgrade` does NOT rotate by default — that would break the running inadyn. Pass `--rotate-secret` explicitly.

Non-UniFi modes never generate the shared secret (it's serve-mode only).

## Shell installer — final shape

The ~450-line shell installer collapses to ~40 lines because all orchestration moves into the Go binary. The shell job is download+verify+exec, nothing else.

```bash
#!/usr/bin/env bash
# dddns installer — downloads the latest release and hands off to `dddns install`.
set -euo pipefail

GITHUB_REPO="descoped/dddns"
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64)  ARCH=arm64 ;;
  x86_64|amd64)   ARCH=amd64 ;;
  armv7l)         ARCH=armv7 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
          | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
BASE="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
ARCHIVE="dddns_$(tr '[:lower:]' '[:upper:]' <<<"${OS:0:1}")${OS:1}_${ARCH}.tar.gz"
TMP=$(mktemp -d); trap "rm -rf $TMP" EXIT

curl -fsSL "${BASE}/${ARCHIVE}" -o "${TMP}/${ARCHIVE}"
curl -fsSL "${BASE}/checksums.txt" -o "${TMP}/checksums.txt"
( cd "$TMP" && sha256sum -c --ignore-missing checksums.txt 2>&1 | grep -E "${ARCHIVE}: OK" ) \
  || { echo "SHA-256 mismatch" >&2; exit 1; }

tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"
install -m 755 "${TMP}/dddns" /usr/local/bin/dddns

exec /usr/local/bin/dddns install "$@"
```

That's it. Everything else the shell used to do — platform probe, mode prompt, boot script generation, cron entry install, systemd unit write, token print, unifios-utilities bootstrap — moves into Go.

## What stays unchanged

Explicitly preserved per "don't remove any":

- Every existing subcommand: `update`, `ip`, `verify`, `serve` (+ `status`, `test`), `config` (+ `init`, `check`, `set-mode`, `rotate-secret`), `secure` (+ `enable`, `test`).
- Every existing flag on those subcommands.
- The `.secure` at-rest encryption path.
- The device-derived key derivation (weakness noted in `4_security-roadmap.md`; not re-designing here).
- The systemd unit hardening (`ProtectSystem=strict`, `NoNewPrivileges`, etc.).
- The L1–L6 security model for serve mode.
- The audit log JSONL format + 10 MB rotation.

New additions are purely additive:

- `dddns install` — the one-command bootstrap.
- `dddns doctor` — the diagnostic.
- `dddns uninstall` — thin wrapper over the removal steps (replaces the shell `--uninstall` flag; shell installer forwards to it).

## Implementation sequence

1. **New package `internal/install`** with `Run(ctx, opts) error` and per-platform install funcs.
2. **New `cmd/install.go`** — thin cobra wrapper over `internal/install.Run`.
3. **New `cmd/doctor.go`** — read-only health probe.
4. **New `cmd/uninstall.go`** — wraps removal.
5. **Expand `internal/profile`** — add `Scheduler()` returning `{systemd, cron, launchd, none}`.
6. **Expand `internal/bootscript`** — add `renderSystemdTimer(p)` and `renderLaunchd(p)` alongside `renderCron` / `renderServe`.
7. **Shell installer shrinkage** — replace `scripts/install-on-unifi-os.sh` with the 40-line script above. The old script name is kept for URL stability; content is replaced.
8. **Tests**: per-platform probes with `httptest` for the Route53 probe, `testing/fstest` for `/proc/` fixtures, and real idempotency checks (run `install` twice → no diff, no restart).

Each step is a single commit. No step requires more than ~200 lines of Go.

## Exit criteria

- `dddns install` on a fresh UDR with no flags: prompts for 4 fields, installs, runs, prints UniFi UI values. Zero edits to any file by hand.
- `dddns install --upgrade` on an existing UDR: preserves mode, replaces binary, re-runs boot script, no prompts, no secret rotation.
- `dddns doctor` on a working install prints all `✓`. On a broken one, points at the specific fix.
- Shell installer under 50 lines.
- Every existing subcommand still works exactly as before.
- 28 edge cases above all covered by either code or a clear user-facing error string.

## Open questions (I won't invent answers to these — need your call)

1. **Uninstall target:** `dddns uninstall` vs `dddns install --uninstall`? The Go subcommand is clearer; leaving it as an `install` flag keeps the surface smaller. I lean toward `dddns uninstall`. Your call.
2. **Docker mode:** `foreground` assumes the container runtime (systemd-in-container, Kubernetes, plain `docker run`) takes care of restarts. Is that safe to assume, or should Docker support be limited to "`dddns update` from cron via an outer scheduler"?
3. **macOS launchd:** worth supporting? Nobody runs a Mac laptop as a 24/7 DNS updater in production, but a home lab might. Cost: ~60 lines of Go + plist template. Skip unless you ask.
4. **`--rotate-secret` on upgrade:** confirming the default is **off** — user must explicitly opt in. Rotating on every upgrade would break the UniFi inadyn integration every release.
