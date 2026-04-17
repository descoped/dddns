# UniFi-to-Route53 DDNS Bridge

## 1. Overview

A supplementary run mode for dddns on UniFi Dream devices (UDR, UDR7, UDM/UDM-Pro). Instead of cron polling, the UniFi OS built-in `inadyn` client — configured via the Network Controller "Dynamic DNS" UI as a `Custom` service — triggers local HTTP requests to a new `dddns serve` listener on WAN IP changes. The listener runs the same Route53 update path as `dddns update`.

**Mode is exclusive.** A given install runs EITHER `dddns update` (cron) OR `dddns serve` (event-driven), never both. This eliminates cache races and keeps one source of truth. Mode is chosen at install time and switched later with `dddns config set-mode {cron|serve}`.

**Config is shared.** Both modes read the same `config.yaml` / `config.secure`. A new optional `server:` block holds serve-mode parameters.

**Security is non-negotiable.** The listener controls a DNS record for a production domain. The design assumes the shared Basic Auth credential *will* leak at some point (UniFi DB exfil, LAN malware, supply-chain compromise) and is layered so that leakage alone cannot hijack DNS.

## 2. Threat Model

**Asset:** control of the configured A record (e.g. `home.route-66.no`). An attacker who can successfully trigger a Route53 update controls where traffic to that hostname goes.

**In scope:**
- **LAN attackers** — compromised IoT, guest Wi-Fi client, malware on a trusted host. Can reach `127.0.0.1:53353` only by first compromising the router itself, but can reach `0.0.0.0` binds directly.
- **Local processes on UniFi OS** — any process on the router (containers, UniFi services, a future compromise) can hit loopback.
- **UniFi controller database** — stores the custom-DDNS password; leaked via backup export, controller RCE, or account compromise.
- **Replay attackers** — Basic Auth is a bearer; captured once, replayable forever.

**Out of scope (explicitly):**
- Nation-state MITM on AWS public endpoints (`checkip.amazonaws.com`, `route53.amazonaws.com`).
- Root compromise of the router itself — at that point the attacker owns `config.secure`, the device key, and the binary.

**Constraint that shapes the design:** The UniFi UI gives `inadyn` three inputs (username, password, server URL) and speaks only dyndns v2 HTTP Basic Auth for custom providers. We cannot change the wire protocol to HMAC, mTLS, signed nonces, or OAuth. The credential on the wire will be a shared secret in a Basic Auth header. Any stronger scheme requires replacing the trigger mechanism, defeating the point of the design.

**Therefore:** we accept the credential can be stolen and design layers that make *possession of the credential insufficient to cause harm*.

## 3. Security Model (Layered Defenses)

### L1 — Network reachability (reduce who can even attempt)

- Bind defaults to `127.0.0.1:53353`. Loopback only. Reachable only from processes already on the router.
- LAN reachability is explicit opt-in: `server.bind: "0.0.0.0:53353"` with a loud warning from `config set-mode serve`.
- `RemoteAddr` CIDR allowlist is enforced in the handler as defense in depth. Empty `allowed_cidrs` → server refuses to start (fail-closed).
- Port 53353 is unprivileged; no `CAP_NET_BIND_SERVICE` needed.

### L2 — Credential strength (reduce trivial guessing / weak passwords)

- **Generated, never chosen.** The installer creates a 256-bit secret via `crypto/rand` (`hex.EncodeToString` → 64 chars). No user-chosen passwords.
- **Printed once.** The installer prints the secret to stdout exactly once for pasting into the UniFi UI. After that it lives only encrypted in `config.secure` and in the UniFi controller's own DB. Lose it → rotate, never recover.
- **Constant-time compare** against the stored value (`subtle.ConstantTimeCompare`).
- **At-rest encryption** via the existing device key (AES-256-GCM, same mechanism as `aws_credentials_vault`). An attacker who reads `config.secure` off a backup but is not on the originating device cannot decrypt it.
- **Rotation** is a first-class operation: `dddns config rotate-secret`.

### L3 — Auth-failure lockout (defeat online brute force)

- Sliding window in memory: if ≥5 auth failures occur within 60 seconds, the server responds `badauth` without checking the password for the next 5 minutes.
- Every auth failure is logged at WARN severity with `remote_addr` — legitimate inadyn never fails auth, so any failure is suspicious.
- State is per-process (resets on restart), which is acceptable: an attacker forcing a restart has to race the supervisor's 5-second respawn and cannot evade more than one cycle.

### L4 — Upstream blast radius (the biggest win)

This is where a compromised secret stops mattering.

**Authoritative local WAN IP.** The handler does NOT use the `myip` query parameter as an instruction. It reads the router's real current public IP directly from the WAN interface via the OS (`net.InterfaceAddrs()` filtered for the WAN iface, auto-detected or configured via `server.wan_interface`). The local interface state is the same source `inadyn` reads — they agree by construction except during a microsecond-long DHCP transition. The `myip` query param is used only as a sanity check: if it disagrees with the local IP, the discrepancy is logged as an audit anomaly and the local IP is used for the Route53 UPSERT.

**Why local, not `checkip.amazonaws.com`:** the local interface IP is authoritative (UniFi's own view of the WAN), fast (no remote call), attacker-resistant (forging requires root on the router), and available during a WAN flap when outbound connectivity may not be. A remote checkip call is neither more correct nor more secure — just slower and dependent on a third party.

**Consequence:** an attacker with the shared secret can only cause the record to point to the router's actual WAN IP — which is what it already points to. Compromise becomes a nuisance (wasted Route53 API calls, throttled by L3), not a redirection.

**Scoped AWS IAM policy** (see §7). The dddns binary's AWS credentials are limited to UPSERT on a single record name, single type. Even if both the shared secret AND the AWS keys are stolen, the blast radius is one A record.

**Route53 TTL = 300s.** A successful hijack (assuming L4 fails entirely) clears globally in 5 minutes. Already the project default, documented here explicitly as a defensive property.

### L5 — Detection & recovery

- **Structured audit log** at `/var/log/dddns-audit.log`, JSONL, one line per request:
  ```json
  {"ts":"2026-04-17T12:34:56Z","remote":"127.0.0.1","hostname":"home.route-66.no","myip_claimed":"1.2.3.4","myip_verified":"1.2.3.4","auth":"ok","action":"nochg-cache","route53_change_id":""}
  ```
  Rotated at 10 MB (same policy as `dddns.log`).
- **`myip_claimed` ≠ `myip_verified` is a strong signal** that either a misconfigured client or an attack is in flight. Audit log surfaces this immediately.
- **Optional hook** `server.on_auth_failure: "<shell command>"` — fires the command on any auth failure. Lets the user wire up ntfy/Pushover/email without baking delivery into dddns.
- **CloudTrail** on the hosted zone is recommended (docs); surfaces every `ChangeResourceRecordSets` call, independent of dddns.

### L6 — Fail-closed defaults

The server refuses to start if any of:
- `server.bind` missing
- `server.shared_secret` / `secret_vault` missing
- `server.allowed_cidrs` empty
- `cfg.Hostname` empty

## 4. Architecture

```
UniFi OS (UDR7)
   │
   │ WAN IP change (DHCP / PPPoE)
   ▼
inadyn ──HTTP GET──► 127.0.0.1:53353/nic/update?hostname=…&myip=…
                                     │
                                     ▼
                             dddns serve
                               │   │   │
                               ▼   ▼   ▼
                             cidr auth handler
                                         │
                                         ├──► internal/wanip — read local WAN iface
                                         ▼
                                internal/updater
                                         │
                                         ▼
                                 internal/dns (Route53)
```

## 5. Request Flow

1. UniFi OS detects a WAN IP change.
2. `inadyn` issues `GET /nic/update?hostname=%h&myip=%i` with `Authorization: Basic …`.
3. CIDR middleware: `RemoteAddr` in `allowed_cidrs` → else HTTP 403.
4. Auth middleware: lockout check, then constant-time password compare → else `badauth` + record failure.
5. Handler validates:
   - Method is `GET` → else HTTP 405.
   - `hostname` equals `cfg.Hostname` → else `nohost`.
   - `myip` parses to a public IPv4 → else log anomaly (don't reject; we don't trust it anyway).
6. Handler calls `wanip.FromInterface(cfg.Server.WANInterface)` → `localIP`. Reads the WAN interface's current public IPv4 directly from the OS (no network round-trip, no third-party dependency).
7. If the claimed `myip` ≠ `localIP`: log as anomaly; use `localIP` regardless.
8. Handler calls `updater.Update(ctx, cfg, Options{OverrideIP: localIP})` — updater skips its own IP lookup since the caller provided an authoritative value.
9. Handler writes an audit log entry with `myip_claimed`, `localIP`, `auth_outcome`, `action`, and `route53_change_id`.
10. Handler maps `updater.Result.Action` to a dyndns response (§10).

## 6. Config Schema

### `config.yaml` (plaintext)

```yaml
aws_region: us-east-1
aws_access_key: AKIA…
aws_secret_key: …
hosted_zone_id: Z…
hostname: home.route-66.no
ttl: 300
ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false
ip_source: auto                      # auto | local | remote (see §3 L4)

server:
  bind: "127.0.0.1:53353"           # loopback only by default
  shared_secret: "<64 hex chars>"    # plaintext here, vault in .secure
  allowed_cidrs:
    - "127.0.0.0/8"
  audit_log: /var/log/dddns-audit.log
  on_auth_failure: ""                # optional shell hook; empty = disabled
  wan_interface: ""                  # empty = auto-detect; e.g. "eth8" or "pppoe-wan0"
```

`ip_source` defaults:
- `auto` — `local` if UDM profile detected, else `remote`. No regression for existing cron users on laptops/Docker.
- `local` — read the WAN interface directly. Required for serve mode, optional for cron-on-UniFi.
- `remote` — call `checkip.amazonaws.com` (current cron behavior). Needed for non-router platforms.

Serve mode always uses local, regardless of this setting.

LAN-reachable opt-in example:
```yaml
server:
  bind: "0.0.0.0:53353"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "192.168.1.0/24"    # explicit subnet, not the whole RFC1918 space
```

### `config.secure` (device-encrypted)

```yaml
aws_region: us-east-1
aws_credentials_vault: "<base64 enc ak:sk>"
hosted_zone_id: Z…
hostname: home.route-66.no
ttl: 300
ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false

server:
  bind: "127.0.0.1:53353"
  secret_vault: "<base64 enc secret>"
  allowed_cidrs: ["127.0.0.0/8"]
  audit_log: /var/log/dddns-audit.log
  on_auth_failure: ""
  wan_interface: ""
```

## 7. AWS IAM Policy (Mandatory Scoping)

The Route53 IAM user's policy MUST be scoped to the specific record and action. Document and deploy this as the only supported policy for dddns.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ListZoneForLookup",
      "Effect": "Allow",
      "Action": "route53:ListResourceRecordSets",
      "Resource": "arn:aws:route53:::hostedzone/ZXXXXXXXXXXXXX"
    },
    {
      "Sid": "UpsertSingleARecord",
      "Effect": "Allow",
      "Action": "route53:ChangeResourceRecordSets",
      "Resource": "arn:aws:route53:::hostedzone/ZXXXXXXXXXXXXX",
      "Condition": {
        "ForAllValues:StringEquals": {
          "route53:ChangeResourceRecordSetsNormalizedRecordNames": ["home.route-66.no"],
          "route53:ChangeResourceRecordSetsRecordTypes": ["A"],
          "route53:ChangeResourceRecordSetsActions": ["UPSERT"]
        }
      }
    }
  ]
}
```

With this policy, stolen AWS credentials cannot: delete the record, change the TTL, change MX/NS/TXT/CNAME/AAAA, or touch any other record in the zone. They can only UPSERT an A record named exactly `home.route-66.no`. Combined with L4 IP verification, there is effectively nothing useful an attacker can do with them.

This belongs in `docs/AWS_SETUP.md` as the canonical policy — not as an advanced suggestion.

## 8. Package Layout

```
cmd/
├── update.go                [MODIFY: delegate to internal/updater]
├── serve.go                 [NEW: dddns serve + serve status/test]
└── config.go                [MODIFY: interactive wizard, set-mode, rotate-secret]

internal/
├── updater/
│   └── updater.go           [NEW: DRY core update logic]
├── wanip/
│   └── wanip.go             [NEW: local WAN interface IP lookup with auto-detect]
├── server/
│   ├── server.go            [NEW: net.Listen, graceful shutdown, fail-closed checks]
│   ├── handler.go           [NEW: dyndns protocol handler, IP verification]
│   ├── auth.go              [NEW: constant-time compare + lockout window]
│   ├── cidr.go              [NEW: RemoteAddr allowlist]
│   ├── audit.go             [NEW: JSONL audit log writer + rotation]
│   └── status.go            [NEW: serve-status.json for `serve status`]
├── config/
│   ├── config.go            [MODIFY: ServerConfig struct + validation]
│   └── secure_config.go     [MODIFY: encrypt/decrypt server.shared_secret]
├── crypto/
│   └── device_crypto.go     [MODIFY: factor out EncryptString/DecryptString]
├── bootscript/
│   └── bootscript.go        [NEW: generate /data/on_boot.d/20-dddns.sh per mode]
└── installer/
    └── secret.go            [NEW: CSPRNG secret generation]
```

SRP per file. No file has more than one reason to change.

## 9. Prep Refactors (behavior-preserving)

### 9.1 `internal/updater`

Extract update core from `cmd/update.go:runUpdate`. The updater internally discovers the public IP unless explicitly overridden:

```go
package updater

type Options struct {
    Force      bool
    DryRun     bool
    OverrideIP string  // empty = fetch via myip.GetPublicIP(); non-empty = use as-is (testing / serve-mode hand-off)
}

type Result struct {
    Action   string   // "updated" | "nochg-cache" | "nochg-dns" | "dry-run"
    OldIP    string
    NewIP    string
    Hostname string
}

func Update(ctx context.Context, cfg *config.Config, opts Options) (*Result, error)
```

- `cmd/update.go` passes `OverrideIP: customIP` if `--ip` is used; else empty.
- `internal/server/handler.go` fetches `verifiedIP` itself (to build audit log), then passes `OverrideIP: verifiedIP` — avoids the double fetch.
- Cache reads/writes live only in `internal/updater`.

### 9.2 `crypto.EncryptString` / `DecryptString`

```go
func EncryptString(plaintext string) (string, error)
func DecryptString(ciphertext string) (string, error)

func EncryptCredentials(ak, sk string) (string, error) {
    return EncryptString(ak + ":" + sk)
}
```

Same device key, same AES-256-GCM. The server secret encrypts via `EncryptString` directly.

## 10. Dyndns Response Mapping

| Condition                                             | Body          | HTTP |
|-------------------------------------------------------|---------------|------|
| `RemoteAddr` not in `allowed_cidrs`                   | (empty)       | 403  |
| Method ≠ `GET`                                        | (empty)       | 405  |
| Auth failure OR under lockout                         | `badauth\n`   | 200  |
| `hostname` param missing                              | `notfqdn\n`   | 200  |
| `hostname` ≠ `cfg.Hostname`                           | `nohost\n`    | 200  |
| `updater.Update` → `updated`                          | `good <ip>\n` | 200  |
| `updater.Update` → `nochg-cache` or `nochg-dns`       | `nochg <ip>\n`| 200  |
| Route53 error                                         | `dnserr\n`    | 200  |
| Panic (recovered)                                     | `911\n`       | 200  |

The response carries the *verified* IP, never the client-claimed IP. Always HTTP 200 for dyndns-encoded responses.

## 11. CLI Surface

```
dddns serve                              # start the listener (blocks)
dddns serve status                       # print serve-status.json
dddns serve test --hostname X --ip Y     # send a local Basic-Auth'd test request
dddns config set-mode {cron|serve}       # switch modes; rewrites boot script
dddns config rotate-secret               # regenerate server.shared_secret; print once
```

`serve test` reads the shared secret from config, crafts a Basic Auth `GET` to `127.0.0.1:<port>/nic/update`, and prints the response. This is the SSH-debug path — loopback-only is enough.

## 12. Install, Mode Switching, Rotation

### Install prompt

```
dddns update mode:
  1) cron  — poll every 30 minutes  [default]
  2) serve — event-driven via UniFi Dynamic DNS UI
Choose [1]:
```

On `serve`: installer generates a 256-bit secret, writes the `server:` block, prints UI values, and installs the serve-mode boot script.

### Boot script generation

`internal/bootscript` produces `/data/on_boot.d/20-dddns.sh` based on mode:

- **cron**: writes `/etc/cron.d/dddns` with `*/30 * * * * root dddns update …`. No server.
- **serve**: supervised loop (no cron entry):
  ```sh
  (
    while true; do
      /usr/local/bin/dddns serve >> /var/log/dddns-server.log 2>&1
      sleep 5
    done
  ) &
  ```

### Mode switch

`dddns config set-mode {cron|serve}` validates the target mode (serve requires `cfg.Server` populated), regenerates the boot script, removes the artifact for the other mode (`/etc/cron.d/dddns` or the supervisor loop), runs the boot script once. Idempotent.

### Secret rotation

`dddns config rotate-secret`:
1. Generates new 256-bit secret via `crypto/rand`.
2. Re-encrypts into `config.secure`.
3. Prints the new secret exactly once, framed clearly, with instructions to update the UniFi UI.
4. Appends a rotation event to the audit log.

No restart of the server is required — it reads the config file at request time (or on reload signal). Open question: SIGHUP reload, or restart-on-config-change?  → **Decision:** restart on `set-mode`/`rotate-secret` via supervisor respawn. Simpler, and the 5-second gap is negligible for event-driven updates.

## 13. UniFi UI Reference

UniFi Network Controller → Settings → Internet → Dynamic DNS → Create:

| Field     | Value                                                |
|-----------|------------------------------------------------------|
| Service   | `Custom`                                             |
| Hostname  | must equal `cfg.Hostname` (e.g. `home.route-66.no`) |
| Username  | any non-empty string (handler ignores)               |
| Password  | the shared secret printed by the installer          |
| Server    | `127.0.0.1:53353/nic/update?hostname=%h&myip=%i`     |

`inadyn` runs on-device and targets loopback. LAN reachability is not needed for UniFi's trigger.

## 14. Implementation Plan

Ordered, each step an independent commit with passing tests.

### Phase 0 — Pre-existing bug fixes (orthogonal to UniFi feature)

These are bugs discovered during design analysis. They pre-date this work, are independent of the feature, and belong in their own commits so they can be reviewed on their own merits. Landing them before Phase A means the refactors operate on clean code rather than propagating known defects.

**0.1. Fix path manipulation bugs.**
- `cmd/update.go:writeCachedIP` — replace `path[:strings.LastIndex(path, "/")]` with `filepath.Dir(path)`. Current code panics on paths without `/` (returns `-1`) and breaks on Windows.
- `internal/config/config.go:CreateDefault` — replace `path[:len(path)-len("/config.yaml")]` with `filepath.Dir(path)`. Current code assumes the exact filename `config.yaml`; breaks with `--config foo.yaml` or on Windows.
- Tests: table-driven cases — no separator, trailing slash, Windows backslash.
- **Accept:** unchanged on current Unix paths; no panic on edge cases.

**0.2. Fix `IsProxyIP` silent API-failure bug.**
- `internal/commands/myip/myip.go` — add `Status string` to `geoLocation`; return an error when `status != "success"` rather than unmarshalling into `Proxy: false` and silently reporting "not a proxy". Today, an ip-api.com outage or throttle masquerades as a clean "direct connection".
- Tests: mock responses for `success+proxy=true`, `success+proxy=false`, `status=fail` (must error), malformed JSON.
- **Accept:** API failures surface as errors.

**0.3. Fail-fast on malformed config.**
- `cmd/root.go:initConfig` — on `ReadInConfig` errors other than `ConfigFileNotFoundError`, print the error and `os.Exit(1)`. Today it prints to stderr and silently continues, producing a confusing downstream `aws_access_key is required` error for what is actually a YAML syntax problem.
- Tests: pass malformed YAML; assert non-zero exit and a parse-error message.
- **Accept:** users see the real root cause of config problems.

**0.4. Guard empty hostname in Route53 client.**
- `internal/dns/route53.go` — replace `fqdn[len(fqdn)-1] != '.'` with `strings.HasSuffix(fqdn, ".")`. Panics-on-empty becomes graceful handling, defense in depth behind `Validate()`.
- Tests: empty, dotted, undotted hostnames.
- **Accept:** no panic; behavior identical for non-empty input.

**0.5. Soft-fail proxy check** (relevant only to `ip_source: remote`).
- `cmd/update.go` — on `IsProxyIP` error, log a warning and proceed rather than aborting the update. Today an ip-api.com outage (free tier: 45 req/min) halts every cron tick.
- Add optional `proxy_check_strict: bool` config field (default `false` = soft).
- Note: in `ip_source: local` mode there is no proxy to detect — we read the interface directly. Phase G contains the retirement path for `IsProxyIP` entirely.
- Tests: mock `IsProxyIP` returning an error → strict=false proceeds, strict=true aborts.
- **Accept:** third-party outages no longer block DNS updates by default.

### Phase A — Prep refactors (no behavior change)

**A1. Extract `internal/updater`; add context and signals.**
- Move update core out of `cmd/update.go`; new package `internal/updater`.
- `Update(ctx, cfg, Options) (*Result, error)` with `OverrideIP` option.
- `cmd/update.go` shrinks to: IP detect (if no override) → proxy check → `updater.Update` → print.
- **Plumb `context.Context` with a top-level timeout** (default 30s, overridable). Replace all `context.TODO()` in `internal/dns/route53.go` with the passed-in context so Route53 hangs become bounded.
- **Signal handling in `cmd/update.go`**: use `signal.NotifyContext(ctx, SIGINT, SIGTERM)` so cron-killed updates cancel cleanly instead of leaving the cache inconsistent with an in-flight Route53 call.
- Tests: move existing coverage into `internal/updater/updater_test.go`; add `OverrideIP` test; add a timeout test (Route53 mock that blocks → context cancels).
- **Accept:** `dddns update` happy-path identical; hangs bounded; `SIGTERM` exits cleanly.

**A2. Factor `EncryptString` / `DecryptString` in `internal/crypto`.**
- Extract from `EncryptCredentials`; `EncryptCredentials` becomes a one-liner.
- Tests: round-trip test for arbitrary strings; existing credential tests still pass.
- **Accept:** no behavior change; secure config load/save bit-identical.

### Phase B — Config schema (no consumer yet)

**B1. Add `ServerConfig` struct.**
- `internal/config/config.go`: new `ServerConfig` struct with `Bind`, `SharedSecret`, `AllowedCIDRs`, `AuditLog`, `OnAuthFailure`. Field added to `Config` as optional `*ServerConfig`.
- Validation method on `ServerConfig`: CIDRs parseable, bind is `host:port`, secret non-empty.
- Tests: YAML round-trip; validation errors for each missing field.
- **Accept:** existing configs load without the block; new block loads correctly.

**B2. Encrypt/decrypt `server.shared_secret` in secure config.**
- `internal/config/secure_config.go`: new `secret_vault` field; encrypt via `crypto.EncryptString`, decrypt on load.
- Tests: round-trip secret through `.secure`; device-key change → decryption fails.
- **Accept:** `dddns secure enable` preserves the server block and encrypts the secret.

### Phase C — Server core

**C1. `internal/server/cidr.go` + unit tests.**
- `IsAllowed(remoteAddr, cidrs) bool` with IPv4/IPv6 support.
- Tests: loopback, RFC1918, public, malformed.

**C2. `internal/server/auth.go` + unit tests.**
- Constant-time password compare.
- Lockout state: sliding window, 5 failures / 60 seconds → 5-minute block.
- Tests: success, mismatch, lockout trigger, lockout expiry, thread-safety under concurrent requests.

**C3. `internal/server/audit.go` + unit tests.**
- JSONL writer; open-append, size-based rotation at 10 MB; atomic write.
- Tests: concurrent writes serialize correctly, rotation triggers at threshold.

**C4. `internal/wanip` + unit tests.**
- `FromInterface(ifaceName string) (net.IP, error)` — returns the first non-loopback, non-private, non-link-local IPv4 on the named interface. Empty `ifaceName` triggers auto-detection: pick the interface on the default route, filtered for a publicly-routable IPv4.
- Treat CGNAT (`100.64.0.0/10`) as non-usable (CGNAT users can't usefully run dddns anyway).
- Tests: mocked interface list covering dotted-quad IPv4, RFC1918, CGNAT, PPPoE (`ppp0`), no-address states; fake default-route file.
- Integrates with `cmd/update.go` via the `ip_source` config field (see §6).

**C5. `internal/server/handler.go` + unit tests.**
- Parse query, validate method, hostname, (loose) myip sanity.
- Call `wanip.FromInterface(cfg.Server.WANInterface)` → `localIP`.
- If claimed `myip` ≠ `localIP`: record anomaly in the audit entry.
- Call `updater.Update(ctx, cfg, Options{OverrideIP: localIP})`.
- Emit audit log entry; map `Result.Action` to dyndns body.
- Tests: table-driven for every row of §10; mock `wanip` and `updater`.

**C6. `internal/server/server.go` + `cmd/serve.go`.**
- `net.Listen` on `cfg.Server.Bind`; middleware chain: cidr → auth → handler.
- Fail-closed config checks before `Listen`.
- Graceful shutdown on SIGINT/SIGTERM.
- `dddns serve` cobra command.
- Tests: integration test with `httptest.NewServer` over the middleware chain.

### Phase D — Operational commands

**D1. `dddns serve status`.**
- `internal/server/status.go`: reads `/data/.dddns/serve-status.json` (written by handler on each request).
- Prints last-request-at, last-success-at, failure counts, lockout state.
- Tests: status file round-trip.

**D2. `dddns serve test`.**
- Reads shared secret (decrypting `.secure` if needed), crafts Basic Auth `GET`, prints response body and HTTP status.
- Tests: hits a `httptest.NewServer` running the real handler; verifies exit codes per outcome.

**D3. `dddns config rotate-secret`.**
- Generates 256-bit secret via `crypto/rand`.
- Writes back to the currently-loaded config (secure or plain).
- Prints once, with framed output and UI-update instructions.
- Logs rotation to audit log.
- Tests: round-trip, two rotations produce different secrets, audit entry written.

**D4. `internal/bootscript` + `dddns config set-mode`.**
- Pure function: `Generate(mode, paths) string` returns the correct boot script body.
- `set-mode` validates target, writes `/data/on_boot.d/20-dddns.sh`, removes `/etc/cron.d/dddns` or serve supervisor as appropriate, runs the script once.
- Tests: golden-file tests on generated scripts; validates that switching is idempotent.

### Phase E — Installer integration

**E1. `scripts/install-on-unifi-os.sh` — mode prompt + checksum verification.**
- Interactive prompt; `--mode {cron|serve}` flag for non-interactive.
- On `serve`: generate secret (via `dddns config rotate-secret --init`), populate `config.yaml`, print UI values in a clearly-framed block.
- Calls `dddns config set-mode` to write the boot script.
- Idempotent upgrade path preserves existing mode and config.
- **SHA-256 verification of the downloaded binary** against `checksums.txt` from the same GitHub release. Fail the install on mismatch or missing checksum file. Closes a supply-chain gap: GoReleaser already publishes the file; the installer just needs to fetch and verify it (~10 lines of bash).
- Tests: manual on-device; add a negative test that tampers with the downloaded tarball and asserts the installer aborts before extracting.

### Phase F — Documentation

**F1. `docs/AWS_SETUP.md` — scoped IAM.**
- Replace existing IAM guidance with the §7 policy as the *only* supported option.
- Step-by-step IAM user / policy creation with condition keys.
- Explain the blast-radius reduction.

**F2. `docs/UDM_GUIDE.md` — serve mode.**
- New section covering serve-mode install, UI setup, rotation, log files, mode switching.
- Update the "Monitoring" section to cover the audit log and distinguish it from the operational log.

**F3. `docs/TROUBLESHOOTING.md` — serve-mode issues.**
- `badauth` → check UniFi UI password against current secret
- `nohost` → hostname mismatch
- `dnserr` → AWS IAM / connectivity
- `911` → panic, check `/var/log/dddns-server.log`
- Lockout behavior and how to wait it out vs. restart.

### Phase G — Future enhancements (out of scope for v1)

These are deliberately deferred to keep the initial surface small and well-tested. Each is a self-contained follow-up.

- **G1.** Multi-hostname support (depends on the v2 multi-target config work in `ai_docs/0_` and `ai_docs/1_`).
- **G2.** IPv6 / AAAA record support (currently A-only).
- **G3.** Non-root execution — add a `dddns` system user, adjust config file ownership, use `setcap` for any privileged operations. Requires installer rework.
- **G4.** CloudTrail / SNS integration guide in docs (detect out-of-band Route53 changes).
- **G5.** Pluggable notification backend for `on_auth_failure` (ntfy, Slack webhook, SMTP) as a small built-in alternative to shell hooks.
- **G6.** Retire the `ip-api.com` dependency. Once `ip_source: local` is the default for router platforms, the only caller of `IsProxyIP` is `ip_source: remote` on non-router platforms. Consider dropping the proxy check entirely — a returned IP that fails `net.IP.IsGlobalUnicast()` or falls into reserved ranges can be rejected without consulting a third-party API.

### Estimated Scope

| Phase | Commits | Net code added (approx) |
|-------|---------|-------------------------|
| 0 (bug fixes)   | 5  | ~150 lines + tests      |
| A (refactor)    | 2  | ~50 (context + signals) |
| B (schema)      | 2  | ~150 lines              |
| C (server core) | 6  | ~800 lines + tests      |
| D (ops)         | 4  | ~300 lines              |
| E (installer)   | 1  | ~100 bash lines         |
| F (docs)        | 3  | (docs only)             |
| **Total**       | **23 commits** | **~1,550 lines Go + docs** |

No new Go module dependencies (everything uses stdlib + existing deps).
