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
    OverrideIP string  // empty = resolve via cfg.IPSource dispatch; non-empty = use as-is
}

type Result struct {
    Action   string   // "updated" | "nochg-cache" | "nochg-dns" | "dry-run"
    OldIP    string
    NewIP    string
    Hostname string
}

func Update(ctx context.Context, cfg *config.Config, opts Options) (*Result, error)
```

- `cmd/update.go` passes `OverrideIP: customIP` if `--ip` is used; else empty (updater resolves via `cfg.IPSource`).
- `internal/server/handler.go` calls `wanip.FromInterface` itself (to build audit entry with claim-vs-real), then passes `OverrideIP: localIP` to the updater.
- Cache reads/writes live only in `internal/updater`. The internal `resolveIP(cfg)` helper switches on `cfg.IPSource` — `local` → `wanip.FromInterface`, `remote` → `myip.GetPublicIP`, empty/`auto` → mode-based default.

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

**0.1. Fix path manipulation bugs.** ✅ Completed
- `cmd/update.go:writeCachedIP` — replace `path[:strings.LastIndex(path, "/")]` with `filepath.Dir(path)`. Prior code panicked on paths without `/` (LastIndex returned `-1`) and broke on Windows.
- `internal/config/config.go:CreateDefault` — replace `path[:len(path)-len("/config.yaml")]` with `filepath.Dir(path)`. Prior code assumed the exact filename `config.yaml`; broke with `--config foo.yaml` or on Windows.
- Tests added: `TestWriteCachedIP_NestedPath`, `TestWriteCachedIP_RelativePath` (cmd); `TestCreateDefaultConfig_NonStandardFilename` (internal/config). Full suite green (77 tests across 10 packages).

**Status:** ✅ Complete. Commit `99184e9`.
**Findings:** No remaining work. Both bug sites collapsed cleanly to `filepath.Dir`. Existing tests passed unchanged, confirming no behavior difference on valid Unix paths; the three new tests cover the previously-broken shapes (nested, bare filename, non-`config.yaml` name).

**0.2. Fix `IsProxyIP` silent API-failure bug.** ✅ Completed
- `internal/commands/myip/myip.go` — added `Status` and `Message` fields to `geoLocation`; `IsProxyIP` now returns a descriptive error when `status != "success"` instead of unmarshalling into `Proxy: false` and silently reporting "not a proxy".
- Added package var `ipAPIBaseURL` so tests can redirect requests to an `httptest` server; made the URL `?fields=status,message,proxy` so error messages can be surfaced to the caller.
- Tests added (`internal/commands/myip/myip_mock_test.go`, internal package): `TestIsProxyIP_Success_NotProxy`, `TestIsProxyIP_Success_IsProxy`, `TestIsProxyIP_StatusFail`, `TestIsProxyIP_MalformedJSON`, `TestIsProxyIP_EmptyStatus`.
- Updated existing `TestIsProxyIP_InvalidIP` to assert the new error-surfacing behavior. Reshaped `TestGetPublicIP` to exercise only `GetPublicIP` (the IsProxyIP call it previously made was testing the old broken behavior — see finding).

**Status:** ✅ Complete. Full suite green (82 tests across 10 packages).
**Findings — significant:** `ip-api.com` free tier does **not** support HTTPS. Every request we've ever made over HTTPS has returned `{"status":"fail","message":"SSL unavailable for this endpoint, order a key at https://members.ip-api.com/"}` and been silently collapsed to "not a proxy" by the old code. In effect, the proxy check has been a no-op in production for the entire lifetime of dddns. After this fix, the check now correctly errors out — which in cron mode means every `dddns update` run will log `Warning: proxy check failed: ip-api.com request failed: SSL unavailable...`. Users can suppress this by setting `skip_proxy_check: true` in config.
**Follow-up work:** resolved by retiring the entire ip-api.com dependency in the immediately-following commit (see G6 below). The proxy check is now gone, replaced by stdlib-only IP validation.

**0.3. Fail-fast on malformed config.** ✅ Completed
- `cmd/root.go:initConfig` — non-`ConfigFileNotFoundError` errors from `viper.ReadInConfig` now print the underlying message and `os.Exit(1)`. A not-found error is still silently tolerated (first-run scenarios, `dddns config init` on a fresh install).
- Test added: `TestMalformedConfig` in `tests/integration_test.go` — feeds unterminated-quoted YAML via `--config`, asserts non-zero exit, the message contains `Error reading config file`, and the output does *not* contain the old misleading `aws_access_key is required` downstream symptom.

**Status:** ✅ Complete. Full suite green (89 tests across 10 packages).
**Findings:** Straightforward fix. No surprises. Behaviour on malformed-but-recoverable configs (e.g. user wants to reinitialise with `dddns config init`) is unchanged only for the not-found path; a syntactically-invalid existing file blocks all commands including `config init` — the user must delete or rename it first. Documented this implicit contract in the code comment.

**0.4. Guard empty hostname in Route53 client.** ✅ Completed
- `internal/dns/route53.go` — both `GetCurrentIP` and `UpdateIP` now use `strings.HasSuffix(fqdn, ".")` instead of `fqdn[len(fqdn)-1] != '.'`. The prior indexing panicked at runtime when called with an empty hostname; `Config.Validate()` catches this upstream, but the client now handles it safely as defense in depth.
- Tests added: `TestRoute53Client_GetCurrentIP_EmptyHostname`, `TestRoute53Client_UpdateIP_EmptyHostname`, `TestRoute53Client_AlreadyDottedHostname` (asserts no double-dot when the configured hostname already ends with `.`).

**Status:** ✅ Complete. Full suite green (92 tests across 10 packages).
**Findings:** No surprises. The fix was a two-line swap plus adding `strings` to the imports. Behavior on non-empty inputs is identical.

### Phase A — Prep refactors (no behavior change)

**A1. Extract `internal/updater`; add context and signals.** ✅ Completed
- New package `internal/updater` with `Update(ctx, cfg, Options) (*Result, error)`. Options: `Force`, `DryRun`, `Quiet`, `OverrideIP`, `Client` (a `DNSClient` interface the package exposes — lets tests and the future serve handler inject a mock or a shared client). Result: `Action` (`updated`/`nochg-cache`/`nochg-dns`/`dry-run`), `OldIP`, `NewIP`, `Hostname`.
- `cmd/update.go` reduced to: load config, build signal-cancellable + timeout-bounded context, validate `--ip`, delegate. `logInfo`/flag variables removed where the updater owns them.
- `internal/dns/route53.go`: `GetCurrentIP` and `UpdateIP` now take `ctx context.Context`; every `context.TODO()` on a request path is gone. `NewRoute53Client` still uses `context.TODO()` internally for `config.LoadDefaultConfig` — acceptable because that path is local-only with static credentials.
- `cmd/update.go` wraps runs in `signal.NotifyContext(ctx, SIGINT, SIGTERM)` + `context.WithTimeout(ctx, 30s)`. Cron-killed updates cancel cleanly; hangs bounded.
- `cmd/verify.go:r53Client.GetCurrentIP(ctx)` also updated with its own 10s timeout context.
- Tests: `TestReadCachedIP`, `TestWriteCachedIP`, `TestWriteCachedIP_NestedPath`, `TestWriteCachedIP_RelativePath` moved from `cmd/update_test.go` to `internal/updater/updater_test.go`. `cmd/update_test.go` is now a placeholder (the command itself is covered by `tests/integration_test.go`). Added `TestUpdate_OverrideIP`, `TestUpdate_NoChgDNS`, `TestUpdate_NoChgCache`, `TestUpdate_DryRun`, `TestUpdate_ContextTimeout` using an injected `fakeDNSClient` and `blockingDNSClient`. DNS-package tests were updated to pass `context.Background()` to the new signatures.

**Status:** ✅ Complete. Full suite green (97 tests across 11 packages).
**Findings:** The extraction was clean — one `DNSClient` interface in the updater package was enough to make the code testable without touching AWS. `cmd/verify.go` also called `GetCurrentIP()` and needed the same context plumbing; easy to miss without a grep. No behavior change observed in integration tests; cron path still logs the same messages in the same order.

**A2. Factor `EncryptString` / `DecryptString` in `internal/crypto`.** ✅ Completed
- Extracted the AES-256-GCM + base64 machinery into `EncryptString(plaintext)` / `DecryptString(encoded)`. `EncryptCredentials` is now a one-liner wrapping `EncryptString(ak + ":" + sk)`; `DecryptCredentials` wraps `DecryptString` and splits on `:`.
- `TestEncryptDecryptString_RoundTrip` and `TestDecrypt_TamperedCiphertext_Fails` appended to the existing `internal/crypto/device_crypto_test.go` (see finding).

**Status:** ✅ Complete. Full suite green (99 tests across 11 packages).
**Findings:** The plan claimed the crypto package had no direct unit tests — that was wrong. `internal/crypto/device_crypto_test.go` already existed with 7 tests (device-key consistency, credentials round-trip across input shapes, nonce freshness, invalid-data handling, secure-wipe, device-key fallback, plus a skipped different-keys scenario). Appending just the two genuinely new cases (direct round-trip of the new string primitives, and GCM auth-tag detection via a last-byte bit flip) instead of duplicating coverage. Existing tests pass unchanged — the refactor is behavior-preserving.

### Phase B — Config schema (no consumer yet)

**B1. Add `ServerConfig` struct and `IPSource` field.** ✅ Completed
- `internal/config/config.go`: new `ServerConfig` struct with `Bind`, `SharedSecret`, `AllowedCIDRs`, `AuditLog`, `OnAuthFailure`, `WANInterface`. Added as optional `*ServerConfig` on `Config`. Dual struct tags (`mapstructure` + `yaml`) so the same type can be embedded into `SecureConfig` in B2 without duplication.
- `IPSource string` added top-level on `Config`. Accepted values: `""`/`"auto"` (default, mode-driven), `"local"`, `"remote"`. Config.Validate rejects anything else.
- `ServerConfig.Validate()` returns clear errors for: missing bind, bad bind (not `host:port`), missing shared secret, empty allowed_cidrs (fail-closed per §3 L6), and unparseable CIDR entries. Config.Validate intentionally does *not* call it — cron-only users with no `server:` block shouldn't hit serve-mode validation.
- Tests added in `internal/config/config_test.go`: `TestLoadConfig_WithServerBlock` (full round-trip), `TestLoadConfig_NoServerBlock` (existing configs still load; `Server` stays nil), `TestConfigValidate_BadIPSource`, `TestServerConfigValidate` (table-driven with the 5 failure modes above).

**Status:** ✅ Complete. Full suite green (108 tests across 11 packages).
**Findings:** No surprises. The dual `mapstructure`/`yaml` tag approach means one struct covers both config surfaces; B2 can embed it directly without re-declaring fields.

**B2. Encrypt/decrypt `server.shared_secret` in secure config.** ✅ Completed
- New `SecureServerConfig` struct mirrors `ServerConfig` but stores `SecretVault` in place of `SharedSecret`. `SecureConfig` gains `Server *SecureServerConfig` and a top-level `IPSource` field.
- `SaveSecure` now calls `crypto.EncryptString(cfg.Server.SharedSecret)` into `secret_vault`; `LoadSecure` reverses it with `crypto.DecryptString`. `MigrateToSecure` picks this up transparently because it delegates to Save/Load.
- New test file `internal/config/secure_config_test.go` with: `TestSaveLoadSecure_WithServerBlock` (full round-trip), `TestSaveSecure_SecretIsEncryptedAtRest` (grep the on-disk file for a distinctive plaintext marker — must not appear), `TestLoadSecure_NoServerBlock` (prior-shape .secure files still load), `TestLoadSecure_TamperedVault` (prepend bytes to the base64 vault → GCM rejects).

**Status:** ✅ Complete. Full suite green (112 tests across 11 packages).
**Findings:** The tamper test needed `os.Chmod(path, 0600)` before re-writing — `SaveSecure` writes `0400` (read-only by design). Worth noting for any future test that needs to exercise corruption-style scenarios against the on-disk `.secure` file.

### Phase C — Server core

**C1. `internal/server/cidr.go` + unit tests.** ✅ Completed
- New package `internal/server` with `IsAllowed(remoteAddr, cidrs) bool`. Accepts `host:port` or a bare IP, strips the IPv6 `%zone` suffix if present, parses each CIDR with `net.ParseCIDR`, returns true on the first containing network. Fails closed on empty input, unparseable host, or all-malformed CIDRs.
- Table-driven `TestIsAllowed` with 18 subtests: loopback v4 with and without port, each RFC1918 range, public addresses (8.8.8.8, 1.1.1.1), narrower subnet hit/miss, IPv6 loopback, public IPv6 deny, link-local IPv6 with `%zone`, a malformed CIDR entry silently skipped, and all-malformed CIDRs denying.

**Status:** ✅ Complete. Full suite green (130 tests across 12 packages).
**Findings:** Clean drop-in. `net.SplitHostPort` handles `[v6]:port` correctly, so no IPv6 special-casing was needed for the port split.

**C2. `internal/server/auth.go` + unit tests.** ✅ Completed
- New `Authenticator` type wrapping a shared secret + sliding-window lockout state under a `sync.Mutex`. `Check(password)` returns an `AuthResult` enum (`AuthOK`, `AuthBadCredentials`, `AuthLockedOut`) so the handler can map each outcome to a distinct dyndns response without inspecting error strings.
- Lockout constants exported: `MaxFailuresPerWindow=5`, `FailureWindow=60s`, `LockoutDuration=5m`. Password comparison uses `crypto/subtle.ConstantTimeCompare`. A successful auth clears the recent-failures tally so legitimate callers aren't penalised for historical typos.
- `now` is an injectable `func() time.Time` so tests can advance virtual time without sleeping.
- 8 tests cover: success, bad credentials, empty password, lockout trip after threshold, failures outside the sliding window not counting, lockout expiry, success clearing the tally, and a 2,000-call concurrent exerciser validated by `go test -race`.

**Status:** ✅ Complete. `go test -race ./internal/server/...` clean. Full suite green (138 tests across 12 packages).
**Findings:** No surprises. The `fakeClock.advance` pattern keeps lockout tests deterministic and sub-second fast.

**C3. `internal/server/audit.go` + unit tests.** ✅ Completed
- New `AuditLog` type with `AuditEntry` (ts, remote, hostname, myip_claimed, myip_verified, auth, action, route53_change_id, error). `Write` marshals the entry as one JSON line, appends it with `O_APPEND|O_CREATE|O_WRONLY` under a `sync.Mutex`, and rotates the file to `path+".old"` when the existing size hits `AuditMaxSize` (10 MB by default; overridable for tests).
- `now` is injectable (same pattern as `Authenticator`) so the timestamp in each written entry is deterministic in tests.
- 5 tests: basic write + round-trip, multi-line append, rotation at a tiny threshold, a 1,000-write concurrent exerciser (validates no partial/interleaved lines), and a timestamp-hook check.

**Status:** ✅ Complete. `go test -race ./internal/server/...` clean. Full suite green (143 tests across 12 packages).
**Findings:** No surprises. Single-writer `O_APPEND` is atomic for lines well under `PIPE_BUF`; the mutex is belt-and-braces but simplifies the rotation-then-append sequence (otherwise two goroutines could both observe the file over-threshold and both try to rename).

**C4. `internal/wanip` + updater dispatch + unit tests.** ✅ Completed
- New package `internal/wanip`. `FromInterface(ifaceName)` returns the first usable public IPv4 address bound to the interface. `ipNet`/`IPAddr` addrs both supported. Empty `ifaceName` triggers auto-detection by parsing `/proc/net/route` for the 0.0.0.0 destination row.
- `isPublicIPv4` combines `net.IP.IsGlobalUnicast`, `net.IP.IsPrivate`, an IPv6 rejection, and an explicit `100.64.0.0/10` (CGNAT) check.
- Test hooks: package-level `interfaceAddrs` and `defaultRoutePath` variables. 11 tests cover a public dotted-quad, RFC1918-skipped-then-public-found, CGNAT-only rejected, loopback-only rejected, no-addresses, PPPoE (`ppp0` with `/32`), unknown-interface, auto-detect happy path, auto-detect with missing default route, auto-detect with missing file, and a table-driven `isPublicIPv4` check.
- `internal/updater` gains `resolveIP(cfg)` that dispatches: `local` calls a hook around `wanip.FromInterface`, `remote` calls a hook around `myip.GetPublicIP`, empty/`auto` picks local on the UDM profile and remote elsewhere. The hooks are package-level `var`s so dispatch tests don't hit the network.
- 5 dispatch tests in `internal/updater/updater_test.go` cover explicit local (and WANInterface pass-through), explicit remote overriding UDM-default, auto on UDM → local, auto off UDM → remote, and an invalid `ip_source` value.

**Status:** ✅ Complete. Full suite green (159 tests across 13 packages).
**Findings:** cron-on-UDM now reads the WAN interface locally by default — zero network round-trip for 99% of users. Cron on laptops/Docker keeps calling `checkip.amazonaws.com` per the `remote` default for non-UDM profiles; nothing changed for them. The package-level hook pattern (mirroring `Authenticator.now` and `AuditLog.now`) keeps the resolver pure and fully mockable.

**C5. `internal/server/handler.go` + `internal/server/status.go` (writer) + unit tests.** ✅ Completed
- New `StatusWriter` serializes a `StatusSnapshot` (last_request_at, last_remote_addr, last_auth_outcome, last_action, last_error) to the configured path via tempfile-and-rename. Writes are atomic — a concurrent reader never sees a partial file.
- New `Handler` implements `http.Handler`. Per-request it: (1) CIDR-gates on `RemoteAddr` (HTTP 403); (2) rejects non-GET (HTTP 405); (3) does Basic-Auth via `Authenticator.Check` (→ `badauth` on miss or lockout); (4) validates `hostname` (→ `notfqdn`/`nohost`); (5) reads the local WAN IP with `wanip.FromInterface`; (6) delegates to `updater.Update` with `OverrideIP=localIP.String()` so the `myip` query param is *never* trusted for the upsert — it's only captured in the audit entry; (7) maps `Result.Action` to `good`/`nochg`/`dnserr`; (8) recovers from panics → `911`. Each request wraps the outgoing call in `context.WithTimeout(r.Context(), 30s)` to bound Route53 hangs. Every outcome (including rejections) emits an audit line and refreshes the status file.
- Test fixture in `handler_test.go` uses `httptest.NewRecorder` and an injected stub for `wanIP` and `updateIP` — no network, no AWS. 13 tests cover: §10 response table (CIDR deny, method 405, missing/bad auth, missing hostname, wrong hostname, updated=good, nochg, route53 error, wanip error), lockout fall-through to `badauth`, the anomaly case (myip claim ≠ local IP) where the handler pushes local anyway and records both in the audit entry, the status file being refreshed with the correct fields, and a concurrent atomic-replace check on `StatusWriter`.

**Status:** ✅ Complete. `go test -race ./internal/server/...` clean. Full suite green (173 tests across 13 packages).
**Findings:** Keeping CIDR + auth in the handler (instead of separate `http.Handler` middleware) let me build one `AuditEntry` across every outcome and emit it from a single point. A later refactor can split middleware if needed; current shape is simple and tested end-to-end.

**C6. `internal/server/server.go` + `cmd/serve.go`.** ✅ Completed
- New `Server` type wraps `http.Server` with a constructor that validates both `Config` and `ServerConfig` (fail-closed per L6) and constructs all the per-request dependencies (`Authenticator`, `AuditLog`, `StatusWriter`, `Handler`). The handler is mounted at `/nic/update` on an `http.ServeMux`. HTTP timeouts: `ReadHeaderTimeout=5s`, `ReadTimeout=10s`, `WriteTimeout=35s` (must exceed the handler's 30s budget), `IdleTimeout=30s`.
- `Server.Run(ctx)` blocks until ctx is cancelled, then calls `http.Server.Shutdown` with a 5-second deadline to drain in-flight requests. Clean exit on both `SIGINT` and `SIGTERM`.
- `cmd/serve.go` wires the cobra `dddns serve` command: loads config, constructs the server, wraps ctx with `signal.NotifyContext`, calls `Run`.
- Paths: the audit log honours `cfg.Server.AuditLog` if set, otherwise falls back to `<cfg.IPCacheFile dir>/serve-audit.log`. The status file is always `<cfg.IPCacheFile dir>/serve-status.json`.
- 6 new tests in `server_test.go`: constructor with a valid config, constructor rejecting an invalid top-level config, missing server block, invalid server block; an end-to-end integration test that spins up `httptest.NewServer` over the wired handler chain and asserts `good 81.191.174.72` comes back over a real HTTP socket; a graceful-shutdown test that binds an ephemeral port, serves a request, cancels the context, and verifies `Run` returns within 2s without error.

**Status:** ✅ Complete. `go test -race ./internal/server/...` clean. Full suite green (179 tests across 13 packages). `dddns serve` appears in the CLI help.
**Findings:** The `Server.binder` indirection turned the graceful-shutdown test from "start, guess the port, hope" into a deterministic ephemeral-port bind. Worth keeping — a future signal-reload test will also need it.

### Phase D — Operational commands

**D1. `dddns serve status` + `internal/server/status.go` (reader).**
- Add `Read()` helper to `status.go` (writer created in C5).
- Prints last-request-at, last-success-at, failure counts, lockout state from the status file.
- Tests: status file round-trip (write via the C5 helper, read via the subcommand).

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
- ~~**G6.** Retire the `ip-api.com` dependency.~~ ✅ Completed. `IsProxyIP`, `geoLocation`, `toJSON`, the `ip-api.com` base URL, and the `skip_proxy_check` config field were all removed. Replaced by `myip.ValidatePublicIP`, a stdlib-only helper that rejects malformed, IPv6, loopback, link-local, unspecified, multicast, and private (RFC1918) addresses. The `--check-proxy` flag on `dddns ip` was removed along with the feature.

### Estimated Scope

| Phase | Commits | Net code added (approx) |
|-------|---------|-------------------------|
| 0 (bug fixes)   | 4  | ~130 lines + tests      |
| A (refactor)    | 2  | ~50 (context + signals) |
| B (schema)      | 2  | ~150 lines              |
| C (server core) | 6  | ~800 lines + tests      |
| D (ops)         | 4  | ~300 lines              |
| E (installer)   | 1  | ~100 bash lines         |
| F (docs)        | 3  | (docs only)             |
| **Total**       | **22 commits** | **~1,530 lines Go + docs** |

No new Go module dependencies (everything uses stdlib + existing deps).
