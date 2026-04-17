# UniFi-to-Route53 DDNS Bridge

## 1. Overview

A supplementary run mode for dddns on UniFi Dream devices (UDR, UDR7, UDM/UDM-Pro). Instead of cron polling, the UniFi OS built-in `inadyn` client — configured via the Network Controller "Dynamic DNS" UI as a `Custom` service — triggers local HTTP requests to a new `dddns serve` listener on WAN IP changes. The listener runs the same Route53 update path as `dddns update`.

**Mode is exclusive.** A given install runs EITHER cron-driven `dddns update` OR event-driven `dddns serve`, never both. This eliminates cache races and keeps a single source of truth. Mode is chosen at install time and switched later with `dddns config set-mode {cron|serve}`.

**Config is shared.** Both modes read the same `config.yaml` or `config.secure`. A new optional `server:` block holds serve-mode parameters.

## 2. Architecture

```
UniFi OS (UDR7)
   │
   │ WAN IP change (DHCP / PPPoE)
   ▼
inadyn ──HTTP GET──► 0.0.0.0:53353/nic/update?hostname=…&myip=…
                                     │
                                     ▼
                             dddns serve
                               │   │   │
                               ▼   ▼   ▼
                             cidr auth handler
                                         │
                                         ▼
                                internal/updater
                                         │
                                         ▼
                                 internal/dns (Route53)
```

**Binding.** `0.0.0.0:53353`. LAN-reachable so SSH-in-from-LAN debugging works without tunneling. Port 53353 is unprivileged and unlikely to collide with UniFi services. UniFi UI's "Server" field is a free-form URL template — any port is accepted there; inadyn uses whatever port is supplied.

**Network filter.** The handler rejects any request whose `RemoteAddr` is not in the configured CIDR allowlist. Defaults: `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`. WAN-sourced requests always fail this check. This is defense in depth alongside UniFi's default WAN-inbound firewall block.

**Authentication.** `inadyn` sends `Authorization: Basic base64(user:pass)`. The username is ignored (UniFi UI mandates a value; semantics are caller-defined). The password is compared to the configured shared secret with `subtle.ConstantTimeCompare`.

**Secret storage.** Follows existing convention. Plaintext in `config.yaml` (same as `aws_access_key`); device-encrypted in `config.secure` via the existing AES-256-GCM/device-key path.

## 3. Request Flow

1. UniFi OS detects a WAN IP change.
2. `inadyn` issues `GET /nic/update?hostname=%h&myip=%i` with `Authorization: Basic …`.
3. Server checks `RemoteAddr` against allowed CIDRs → HTTP 403 on miss.
4. Auth middleware verifies Basic Auth password against `cfg.Server.SharedSecret` → `badauth` on miss.
5. Handler validates:
   - Method is `GET` → else HTTP 405.
   - `hostname` equals `cfg.Hostname` → else `nohost`.
   - `myip` parses as public IPv4 (rejects RFC1918, loopback, link-local, unspecified) → else `notfqdn`.
6. Handler calls `updater.Update(ctx, cfg, myip, Options{})`.
7. The `updater.Result.Action` is mapped to a dyndns response (§7).

## 4. Config Schema

### `config.yaml` (plaintext)

```yaml
aws_region: us-east-1
aws_access_key: AKIA…
aws_secret_key: …
hosted_zone_id: Z…
hostname: home.example.com
ttl: 300
ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false

server:
  bind: "0.0.0.0:53353"
  shared_secret: "…"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

### `config.secure` (device-encrypted)

```yaml
aws_region: us-east-1
aws_credentials_vault: "<base64 enc ak:sk>"
hosted_zone_id: Z…
hostname: home.example.com
ttl: 300
ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false

server:
  bind: "0.0.0.0:53353"
  secret_vault: "<base64 enc secret>"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

Presence of `server:` enables serve mode to be selectable; the actual active mode is set by `dddns config set-mode`.

## 5. Package Layout

```
cmd/
├── update.go                [MODIFY: delegate to internal/updater]
├── serve.go                 [NEW: dddns serve + status/test subcommands]
└── config.go                [MODIFY: interactive wizard handles server block + set-mode]

internal/
├── updater/
│   └── updater.go           [NEW: DRY core update logic]
├── server/
│   ├── server.go            [NEW: net.Listen, graceful shutdown]
│   ├── handler.go           [NEW: dyndns protocol handler]
│   ├── auth.go              [NEW: constant-time shared-secret check]
│   ├── cidr.go              [NEW: RemoteAddr allowlist]
│   └── status.go            [NEW: serve-status.json read/write]
├── config/
│   ├── config.go            [MODIFY: add ServerConfig]
│   └── secure_config.go     [MODIFY: encrypt/decrypt server.shared_secret]
└── crypto/
    └── device_crypto.go     [MODIFY: factor out EncryptString/DecryptString]
```

Each file under `internal/server/` has one reason to change (SRP).

## 6. Prep Refactors

### 6.1 `internal/updater` (DRY)

`cmd/update.go:runUpdate` currently inlines cache read, Route53 client creation, current-IP check, upsert, and cache write. Extract into a pure function used by both run modes:

```go
package updater

type Options struct {
    Force  bool
    DryRun bool
}

type Result struct {
    Action   string   // "updated" | "nochg-cache" | "nochg-dns" | "dry-run"
    OldIP    string
    NewIP    string
    Hostname string
}

func Update(ctx context.Context, cfg *config.Config, newIP string, opts Options) (*Result, error)
```

- `cmd/update.go` becomes IP detection + proxy check + `updater.Update` + human-readable print.
- `internal/server/handler.go` calls `updater.Update` and maps `Result.Action` to a dyndns code. No proxy check (the router's WAN IP isn't proxied in the dddns sense).
- Cache I/O lives only in `internal/updater`.

### 6.2 `crypto.EncryptString` / `DecryptString`

Factor single-string primitives out of the existing `EncryptCredentials`, which becomes a one-line wrapper:

```go
func EncryptString(plaintext string) (string, error)
func DecryptString(ciphertext string) (string, error)

func EncryptCredentials(ak, sk string) (string, error) {
    return EncryptString(ak + ":" + sk)
}
```

Same device key, same AES-256-GCM. The server secret uses `EncryptString` directly.

## 7. Dyndns Response Mapping

| Condition                                             | Body          | HTTP |
|-------------------------------------------------------|---------------|------|
| `RemoteAddr` not in `allowed_cidrs`                   | (empty)       | 403  |
| Method ≠ `GET`                                        | (empty)       | 405  |
| Basic Auth missing or wrong password                  | `badauth\n`   | 200  |
| `hostname` param missing                              | `notfqdn\n`   | 200  |
| `hostname` ≠ `cfg.Hostname`                           | `nohost\n`    | 200  |
| `myip` unparseable, private, loopback, or link-local  | `notfqdn\n`   | 200  |
| `updater.Update` → `updated`                          | `good <ip>\n` | 200  |
| `updater.Update` → `nochg-cache` or `nochg-dns`       | `nochg <ip>\n`| 200  |
| Route53 error                                         | `dnserr\n`    | 200  |
| Panic (recovered)                                     | `911\n`       | 200  |

Always HTTP 200 for dyndns-encoded responses; semantics are in the body, per protocol. HTTP error codes are reserved for pre-protocol rejections (network origin, wrong method).

## 8. CLI Surface

```
dddns serve                                  # start the listener (blocks)
dddns serve status                           # print /data/.dddns/serve-status.json
dddns serve test --hostname X --ip Y         # send a local test request
dddns config set-mode {cron|serve}           # switch modes; rewrites boot script
```

`serve status` reads the status file that the handler writes on each request: last request timestamp, last successful update, last error, request counts. No `/status` HTTP endpoint — the CLI is the interface.

`serve test` reads the shared secret from config, crafts a Basic Auth `GET` to `127.0.0.1:<port>/nic/update`, and prints the response. This is the SSH debugging path.

## 9. Install & Mode Switching

### Install prompt

```
dddns update mode:
  1) cron  — poll every 30 minutes  [default]
  2) serve — event-driven via UniFi Dynamic DNS UI
Choose [1]:
```

On `serve` selection, the installer generates a random shared secret (`openssl rand -hex 16`), writes the `server:` block, and prints the UniFi UI values to paste.

### Boot script

The installer writes ONE script at `/data/on_boot.d/20-dddns.sh`:

**Cron mode:** installs `/etc/cron.d/dddns` with `*/30 * * * * root dddns update …`. No server started.

**Serve mode:** launches a supervised loop, no cron entry:
```sh
(
  while true; do
    /usr/local/bin/dddns serve >> /var/log/dddns-server.log 2>&1
    sleep 5
  done
) &
```

Separate log file (`dddns-server.log` vs `dddns.log`) so the two modes never share state.

### Switching modes post-install

`dddns config set-mode {cron|serve}` rewrites `/data/on_boot.d/20-dddns.sh` and runs it once. Idempotent. Switching to `serve` requires `cfg.Server` to be populated (error otherwise). Switching to `cron` leaves `cfg.Server` intact so the user can switch back without re-entering the secret.

### UniFi UI reference (serve mode)

UniFi Network Controller → Settings → Internet → Dynamic DNS → Create:

| Field     | Value                                                |
|-----------|------------------------------------------------------|
| Service   | `Custom`                                             |
| Hostname  | must equal `cfg.Hostname` (e.g. `home.example.com`)  |
| Username  | any non-empty string (handler ignores)               |
| Password  | the shared secret printed by the installer          |
| Server    | `127.0.0.1:53353/nic/update?hostname=%h&myip=%i`     |

`inadyn` runs on-device, so the Server field targets loopback. The LAN bind is only for SSH/debug.

## 10. Implementation Sequence

Each step is an independent commit; tests pass at every point.

1. **Refactor** `internal/updater` — behavior-preserving extraction from `cmd/update.go`.
2. **Refactor** `crypto.EncryptString` / `DecryptString` — factor from `EncryptCredentials`.
3. **Schema** add `ServerConfig` + `secret_vault` field; no consumer yet.
4. **Feature** `internal/server` package + `dddns serve` command.
5. **Feature** `dddns serve status` and `dddns serve test` subcommands.
6. **Feature** `dddns config set-mode` + boot-script generator.
7. **Installer** `install-on-unifi-os.sh` mode prompt, secret generation, UI values output.
