# Architecture

Applies to: whole project.

## Core Flow — three deployment forms, selected by the operator

**Cron mode** (polling, on-device):
1. Resolve current public IP via `cfg.IPSource` — `remote` calls `checkip.amazonaws.com`, `local` reads the WAN interface, `auto` picks based on platform.
2. Compare with cached IP from last run.
3. Update Route53 A record if changed.
4. Cache new IP and exit.
5. Output piped through `logger -t dddns` to systemd-journald.

**Serve mode** (event-driven, on-device same-host bind):
1. `dddns serve` listens on `127.0.0.1:53353` for dyndns v2 requests from a same-host DDNS client.
2. Per request: CIDR check → constant-time Basic Auth → hostname match → read local WAN IP (never trust `myip` query param) → Route53 UPSERT → audit log.
3. Supervised by systemd; restart-on-failure.
4. **Works on Pi / Linux / Docker; experimental on UniFi Dream** — see `serve-mode.md` for the inadyn-binding caveat that rules UniFi Dream out.

**Lambda mode** (event-driven, cloud):
1. `deploy/aws-lambda/` package runs behind API Gateway HTTP API.
2. Per request: Basic Auth against SSM-stored shared secret (60 s cache) → constant-time compare → hostname match → publish API Gateway's `requestContext.http.sourceIp` (ignore `myip=` query param entirely) → Route53 UPSERT.
3. OpenTofu module at `deploy/aws-lambda/tofu/` provisions Lambda + API Gateway + IAM + SSM + CloudWatch.
4. The right choice when UniFi's built-in inadyn is the push source — see `serve-mode.md` for why loopback serve mode doesn't work there.

## Project Structure

```
cmd/                           # CLI commands
├── root.go                   # Main command setup, config loading
├── config.go                 # Config init/check commands
├── config_set_mode.go        # dddns config set-mode {cron|serve}
├── config_rotate_secret.go   # dddns config rotate-secret
├── ip.go                     # IP display command
├── update.go                 # Thin shim over internal/updater
├── verify.go                 # DNS verification
├── secure.go                 # Secure config management
└── serve.go                  # dddns serve / serve status / serve test

internal/
├── bootscript/               # Generates /data/on_boot.d/20-dddns.sh per mode
├── config/                   # YAML + encrypted config, ServerConfig, IPSource
├── crypto/                   # AES-256-GCM + EncryptString/DecryptString primitives
├── dns/                      # Route53 client + hand-rolled SigV4 signer
├── profile/                  # OS/device-specific paths
├── server/                   # Serve-mode HTTP handler (auth, cidr, audit, status)
├── updater/                  # Extracted update flow; DNSClient interface
├── wanip/                    # Local WAN IP with auto-detect (rejects RFC1918/CGNAT/v6)
├── commands/myip/            # Public IP detection + ValidatePublicIP
├── constants/                # Shared constants
└── version/                  # Build-time version injection

deploy/                       # Deployment forms
└── aws-lambda/               # Lambda + API GW + tofu module — see rules/lambda.md
```

## Security invariants (all modes)

- Device-specific AES-256-GCM encryption (cron/serve on-device config), hardware-ID-derived key (MAC / UUID / serial).
- Hand-rolled SigV4 signer with STS session-token support — shared between Route53 and SSM, zero `aws-sdk-go-v2` dependency.
- Constant-time Basic Auth compare; serve mode adds sliding-window lockout.
- L6 defense in both serve and Lambda — `myip=` query param never trusted; ground-truth IP used.
- Lambda IAM scoped to one zone + one record + UPSERT-only + one SSM parameter ARN (see `rules/lambda.md`).
- File permissions: `config.yaml` 0600 enforced at load; `config.secure` 0400.
