# Serve Mode

Applies to: `internal/server/**`, `cmd/serve.go`, `cmd/config_set_mode.go`, `cmd/config_rotate_secret.go`, `internal/bootscript/**`.

## What it is

`dddns serve` is an on-device, systemd-supervised HTTP listener for dyndns v2 push. Bind: `127.0.0.1:53353` (same-host only). Receives requests from a local DDNS client (e.g., inadyn on the same machine), verifies Basic Auth, reads the local WAN IP itself, and UPSERTs Route53.

## Request path

1. CIDR allowlist on `RemoteAddr` — must be loopback unless the operator widens.
2. Constant-time Basic Auth compare against the encrypted local secret.
3. Sliding-window lockout on repeated auth failures.
4. Hostname match against config.
5. Read local WAN IP via `internal/wanip` — **never** trust the `myip` query parameter (L6 defense).
6. Route53 UPSERT via shared `internal/dns` client.
7. Append JSONL audit record (`internal/server/audit.go`) + update `serve-status.json`.

## Supervision model

- systemd on UniFi UDR/UDM (PID 1 on OS 2.x/3.x/4.x); `Restart=on-failure`.
- No in-process supervision, restart loops, or shell-supervisor wrappers — that's systemd's job.
- `dddns config set-mode {cron|serve}` and `dddns config rotate-secret` both restart the supervisor; there is no hot reload.

## Secret rotation (serve mode)

Command: `dddns config rotate-secret [--init]`

Operates on the encrypted local `config.secure` (device-derived AES-256-GCM key). Generates a new secret, re-encrypts the file, writes 0400, prints the value once for pasting into the DDNS client's password field, and restarts the supervisor so the new value is loaded. No SSM involved.

## UniFi Dream caveat

UniFi's built-in `inadyn` binds with `-b eth4`, which routes `127.0.0.1` through the WAN policy table and cannot reach the loopback listener. Serve mode therefore works on Pi / Linux / Docker with a same-host DDNS client, but **Lambda mode is the correct choice** when UniFi's built-in inadyn is the push source.

## Memory-leak regression

`BenchmarkHandler_HappyPath` tracks allocs/op across releases. If a change meaningfully increases allocations, treat it as a regression and justify in the PR.
