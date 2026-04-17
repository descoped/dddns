# UniFi-to-Route53 DDNS Bridge

**Status:** Shipped — on `main` since 2026-04-17. Describes the design as implemented.
**Confidence:** High — reference for shipped code; per-step implementation history lives in git log.
**Last reviewed:** 2026-04-17

## 1. Overview

A supplementary run mode for dddns on UniFi Dream devices (UDR, UDR7, UDM/UDM-Pro). Instead of cron polling, the UniFi OS built-in `inadyn` client — configured via the Network Controller "Dynamic DNS" UI as a `Custom` service — triggers local HTTP requests to a `dddns serve` listener on WAN IP changes. The listener runs the same Route53 update path as `dddns update`.

**Mode is exclusive.** A given install runs EITHER `dddns update` (cron) OR `dddns serve` (event-driven), never both. This eliminates cache races and keeps one source of truth. Mode is chosen at install time and switched later with `dddns config set-mode {cron|serve}`.

**Config is shared.** Both modes read the same `config.yaml` / `config.secure`. A `server:` block holds serve-mode parameters.

**Security is non-negotiable.** The listener controls a DNS record for a production domain. The design assumes the shared Basic Auth credential *will* leak at some point (UniFi DB exfil, LAN malware, supply-chain compromise) and is layered so that leakage alone cannot hijack DNS.

## 2. Threat Model

**Asset:** control of the configured A record (e.g. `home.example.com`). An attacker who can successfully trigger a Route53 update controls where traffic to that hostname goes.

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
  {"ts":"2026-04-17T12:34:56Z","remote":"127.0.0.1","hostname":"home.example.com","myip_claimed":"1.2.3.4","myip_verified":"1.2.3.4","auth":"ok","action":"nochg-cache","route53_change_id":""}
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
hostname: home.example.com
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
hostname: home.example.com
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
          "route53:ChangeResourceRecordSetsNormalizedRecordNames": ["home.example.com"],
          "route53:ChangeResourceRecordSetsRecordTypes": ["A"],
          "route53:ChangeResourceRecordSetsActions": ["UPSERT"]
        }
      }
    }
  ]
}
```

With this policy, stolen AWS credentials cannot: delete the record, change the TTL, change MX/NS/TXT/CNAME/AAAA, or touch any other record in the zone. They can only UPSERT an A record named exactly `home.example.com`. Combined with L4 IP verification, there is effectively nothing useful an attacker can do with them.

This belongs in `docs/aws-setup.md` as the canonical policy — not as an advanced suggestion.

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
- **serve**: installs a systemd unit (`/etc/systemd/system/dddns.service`) with `Restart=always`, `RestartSec=5`, journald logging, and standard hardening (`NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths=/data/.dddns /var/log`). The boot script writes the unit file, runs `systemctl daemon-reload && systemctl enable --now dddns.service`, and removes any stale `/etc/cron.d/dddns` first. `/etc/systemd/system/` is on the firmware-upgrade-wiped root FS, but the boot script re-runs on every boot (via the `udm-boot`/`unifios-utilities` hook) and re-installs the unit — same persistence pattern as cron.

### Mode switch

`dddns config set-mode {cron|serve}` validates the target mode (serve requires `cfg.Server` populated), regenerates the boot script, removes the artifact for the other mode (`/etc/cron.d/dddns` or the supervisor loop), runs the boot script once. Idempotent.

### Secret rotation

`dddns config rotate-secret`:
1. Generates new 256-bit secret via `crypto/rand`.
2. Re-encrypts into `config.secure`.
3. Prints the new secret exactly once, framed clearly, with instructions to update the UniFi UI.
4. Appends a rotation event to the audit log.

No restart of the server is required — it reads the config file at request time (or on reload signal). Open question: SIGHUP reload, or restart-on-config-change? → **Decision:** restart on `set-mode`/`rotate-secret` via supervisor respawn. Simpler, and the 5-second gap is negligible for event-driven updates.

## 13. UniFi UI Reference

UniFi Network Controller → Settings → Internet → Dynamic DNS → Create:

| Field     | Value                                                |
|-----------|------------------------------------------------------|
| Service   | `Custom`                                             |
| Hostname  | must equal `cfg.Hostname` (e.g. `home.example.com`) |
| Username  | any non-empty string (handler ignores)               |
| Password  | the shared secret printed by the installer          |
| Server    | `127.0.0.1:53353/nic/update?hostname=%h&myip=%i`     |

`inadyn` runs on-device and targets loopback. LAN reachability is not needed for UniFi's trigger.
