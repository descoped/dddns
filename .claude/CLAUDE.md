# CLAUDE.md

This file provides guidance to Claude Code when working with dddns.

## CRITICAL RULES - MUST READ

**NEVER COMMIT WITHOUT EXPLICIT PERMISSION**
- When the user says "do not commit" or indicates this is a planning/design session, you MUST NOT make any commits
- Always wait for explicit approval before making design decisions or architectural changes
- The developer (user) is the decision-maker - Claude Code is an assistant, not a decision-maker
- Present options and recommendations, but let the user choose the approach
- If in doubt, ASK before making changes

## Project Overview

**dddns** (Descoped Dynamic DNS) is a lightweight CLI tool for updating AWS Route53 DNS A records with dynamic IP addresses. Originally created to replace a bash script on UDM7 routers, it has evolved into a cross-platform solution while maintaining its core simplicity.

## Development Principles

**DO AS LITTLE AS POSSIBLE TO MAKE IT WORK**
- No over-engineering
- No extra features unless explicitly requested
- Single purpose: update DNS when IP changes
- Optimize for memory-constrained devices (<20MB usage)
- Maintain backward compatibility with existing deployments

## Architecture

### Core Flow
Three deployment forms, selected by the operator:

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
4. **Experimental on UniFi Dream devices** — UniFi's built-in `inadyn` binds with `-b eth4`, which routes `127.0.0.1` through the WAN policy table and cannot reach the listener. Works for Pi / Linux / Docker with a same-host DDNS client.

**Lambda mode** (event-driven, cloud; v0.3.0+):
1. `deploy/aws-lambda/` package runs behind API Gateway HTTP API.
2. Per request: Basic Auth against SSM-stored shared secret (60s cache) → constant-time compare → hostname match → publish API Gateway's `requestContext.http.sourceIp` (ignore `myip=` query param entirely) → Route53 UPSERT.
3. OpenTofu module at `deploy/aws-lambda/tofu/` provisions Lambda + API Gateway + IAM + SSM + CloudWatch.
4. The right choice when a UniFi Dream device's built-in inadyn is the push source — inadyn can reach a public HTTPS endpoint but not a loopback listener.

### Project Structure
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
├── config/                   # Configuration management
│   ├── config.go             # YAML config + ServerConfig + IPSource
│   └── secure_config.go      # Encrypted config + SecureServerConfig (SecretVault)
├── crypto/                   # Security layer
│   └── device_crypto.go      # AES-256-GCM + EncryptString/DecryptString primitives
├── dns/                      # AWS integration
│   └── route53.go            # Route53 client (context-plumbed)
├── profile/                  # Platform detection
│   └── profile.go            # OS/device-specific paths
├── server/                   # Serve-mode HTTP handler
│   ├── server.go             # Lifecycle + fail-closed startup
│   ├── handler.go            # dyndns v2 handler
│   ├── auth.go               # Constant-time + sliding-window lockout
│   ├── cidr.go               # RemoteAddr allowlist
│   ├── audit.go              # JSONL audit log with rotation
│   └── status.go             # serve-status.json writer/reader
├── updater/                  # Extracted update flow
│   └── updater.go            # Update(ctx, cfg, Options); DNSClient interface
├── wanip/                    # Local WAN IP lookup with auto-detect
│   └── wanip.go              # Rejects RFC1918 / CGNAT / link-local / IPv6
├── commands/myip/            # Public IP detection + ValidatePublicIP
├── constants/                # Shared constants
└── version/                  # Build-time version injection

deploy/                        # Deployment forms (v0.3.0+)
└── aws-lambda/
    ├── main.go               # Lambda entry + init-time client construction
    ├── handler.go            # dyndns v2 handler; ignores myip, publishes sourceIp
    ├── ssm.go                # Hand-rolled SSM GetParameter (reuses internal/dns SigV4)
    ├── handler_test.go       # httptest-backed unit tests
    ├── tofu/                 # OpenTofu module (13 resources, all variables)
    ├── scripts/rotate-secret.sh  # openssl rand + ssm put-parameter
    └── README.md             # Operator-facing deploy guide
```

## Key Features Implemented

### Security
- ✅ Device-specific AES-256-GCM encryption (cron/serve on-device config)
- ✅ Hardware ID derivation (MAC, UUID, serial)
- ✅ Secure file permissions (600/400); config.yaml 0600 enforced at load
- ✅ Hand-rolled SigV4 signer with STS session-token support (used by both Route53 and SSM)
- ✅ Constant-time auth compare + (serve) sliding-window lockout
- ✅ Lambda IAM scoped to one zone + one record + UPSERT-only + one SSM parameter ARN
- ✅ L6 defense in both serve and Lambda — `myip=` query param never trusted; ground-truth IP used

### Platform Support
- ✅ UDM/UDR (ARM64, persistent /data storage)
- ✅ Linux (AMD64/ARM64/ARM)
- ✅ macOS (Intel/Apple Silicon)
- ✅ Windows (AMD64/ARM64)
- ✅ Docker containers

### Commands
```bash
# Core commands
dddns update [--dry-run] [--force] [--quiet] [--verbose]
dddns config init
dddns config check
dddns config set-mode {cron|serve}   # Switch run mode; rewrites boot script
dddns config rotate-secret [--init]  # Rotate serve-mode shared secret
dddns ip
dddns verify

# Serve mode (same-host bind)
dddns serve                          # Start listener (blocks)
dddns serve status                   # Show last request / auth outcome
dddns serve test                     # Loopback Basic-Auth'd test request

# Security
dddns secure enable       # Convert to encrypted config
dddns secure test         # Test encryption
```

### UniFi installer actions
```bash
bash install-on-unifi-os.sh                    # Install or upgrade
bash install-on-unifi-os.sh --mode cron        # Fresh install / switch to cron
bash install-on-unifi-os.sh --mode serve       # Fresh install / switch to serve
bash install-on-unifi-os.sh --force            # Reinstall even if version matches
bash install-on-unifi-os.sh --probe            # Privacy-safe self-diagnosis
bash install-on-unifi-os.sh --disable          # Stop update loop; keep binary + config
bash install-on-unifi-os.sh --uninstall        # Remove binary + install dir; preserve config
bash install-on-unifi-os.sh --rollback         # Restore previous version from .prev
```

### AWS Lambda deployment (v0.3.0+)
```bash
just build-aws-lambda                          # Linux arm64 zip for provided.al2023
cd deploy/aws-lambda/tofu && tofu apply        # Creates 13 AWS resources
./scripts/rotate-secret.sh                     # Rotate shared secret; print for UniFi UI
```

## Configuration

### Standard Config (config.yaml)
```yaml
aws_region: "us-east-1"
hosted_zone_id: "ZXXXXXXXXXXXXX"
hostname: "home.example.com"
ttl: 300
access_key: "AKIAXXXXXXXXXXXXXX"    # Optional if using AWS profile
secret_key: "xxxxxxxxxxxxxxxx"      # Optional if using AWS profile
```

### Secure Config (config.secure)
- Encrypted with device-specific key
- Cannot be moved between devices
- Automatic fallback to regular config

## Platform Profiles

| Platform | Config Path | Cache Path | Device ID Source |
|----------|------------|------------|------------------|
| UDM/UDR | `/data/.dddns/` | `/data/.dddns/last-ip.txt` | `/proc/ubnthal/system.info` |
| Linux | `~/.dddns/` | `~/.dddns/last-ip.txt` | `/sys/class/net/eth0/address` |
| macOS | `~/.dddns/` | `~/.dddns/last-ip.txt` | `system_profiler` Hardware UUID |
| Windows | `%APPDATA%/dddns/` | `%APPDATA%/dddns/last-ip.txt` | `wmic` Machine GUID |
| Docker | `/config/` | `/config/last-ip.txt` | Container ID from `/proc/self/cgroup` |

## Build & Release

### Local Development
```bash
just build           # Current platform
just dev             # Race-detector build
just test            # Run tests
just install         # Install to /usr/local/bin
just build-aws-lambda # Linux arm64 zip for AWS Lambda deployment
```
Cross-platform release binaries (UDM arm64, Linux amd64/arm/arm64, macOS,
Windows) are produced by GoReleaser on tag push — see
`.github/workflows/goreleaser.yml`. For a one-off local cross-build, use
`GOOS=... GOARCH=... go build .` directly.

### Release Process
- Uses GoReleaser with git tags
- GitHub Actions automated builds
- Version injection via ldflags
- Multi-platform binaries

### Version Management
```go
// Set at build time via ldflags
var Version = "dev"
var BuildDate = "unknown"
var Commit = "none"
```

## Testing

### Unit Tests (248 tests across 16 packages as of v0.3.0)
- Config loading and validation (`ServerConfig` fail-closed checks)
- IP detection and `ValidatePublicIP` stdlib validator
- Encryption/decryption (`EncryptString`/`DecryptString` round-trip + GCM tamper)
- Platform detection
- Route53 mocking + SigV4 signing (AWS reference-vector tests, with + without session token)
- Serve-mode handler (CIDR / auth / lockout / audit / status — all `httptest`-backed, no AWS)
- Serve-mode memory-leak regression (BenchmarkHandler_HappyPath — tracks allocs/op across releases)
- Lambda handler (auth / hostname / sourceIp / SSM cache — all `httptest`-backed)
- Boot-script generation per mode (including journald pipe for cron mode)

### Integration Tests
- Real AWS API calls (when credentials available)
- File system operations
- Cross-platform compatibility

## CI/CD

### GitHub Actions
- `.github/workflows/ci.yml` - Test, lint, build
- `.github/workflows/goreleaser.yml` - Release automation
- `.github/renovate.json` - Dependency updates

### Quality Checks
- `go vet` - Static analysis
- `golangci-lint` - Extended linting
- Unit test coverage
- Multi-platform build verification

## Deployment

### UniFi installation (cron or serve)
```bash
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh)
```
The installer prompts for run mode on fresh installs, preserves the
detected mode on upgrade, and supports `--mode cron|serve`. Cron mode
pipes output through `logger -t dddns` to systemd-journald — the
legacy `/var/log/dddns.log` is retired as of v0.2.1.

### AWS Lambda deployment
Provisioned via OpenTofu at `deploy/aws-lambda/tofu/`. Three-step flow:
```bash
just build-aws-lambda                                            # build the Lambda zip
cd deploy/aws-lambda/tofu && tofu init && tofu apply             # 13 AWS resources
cd .. && ./scripts/rotate-secret.sh                              # rotate shared secret
# Paste the printed secret into UniFi UI → Dynamic DNS → Password
```
See `deploy/aws-lambda/README.md` for the complete guide.

## Current Status

✅ **Completed**
- Core DNS update functionality
- Cross-platform support (Linux, macOS, Windows, UDM)
- Secure credential storage (AES-256-GCM, device-derived key)
- Hand-rolled SigV4 signer with session-token support (Route53 + SSM)
- Platform auto-detection
- Persistent caching
- UniFi serve mode (event-driven via dyndns-v2 push, systemd-supervised)
- Cron-mode journald routing (`logger -t dddns`, rotation-free)
- `--verbose` flag on `dddns update` for per-step diagnostics
- Installer `--probe` (connectivity + firmware version)
- Installer `--disable` (soft-stop; keeps binary + config)
- Scoped Route53 IAM policy (record-level UPSERT via condition keys)
- Secret rotation (`dddns config rotate-secret` for serve; `deploy/aws-lambda/scripts/rotate-secret.sh` for Lambda)
- Mode switching (`dddns config set-mode {cron|serve}`)
- **AWS Lambda deployment form** (v0.3.0 — API Gateway → Lambda → Route53, full tofu module)
- GoReleaser integration with SHA-256 release verification
- Comprehensive documentation

## Do NOT Add Unless Explicitly Asked

- ❌ Metrics/monitoring/telemetry
- ❌ Web UI or user-facing REST API (the serve-mode `/nic/update` endpoint is a push receiver for inadyn, not a user-facing API)
- ⚠️ Multiple DNS providers — only per the design in `ai_docs/0_provider-architecture.md` (HTTP-only). Do not add a provider outside that framework.
- ❌ Complex retry logic
- ❌ Service discovery
- ❌ Container orchestration
- ❌ Database storage
- ❌ Configuration hot-reload (rotate-secret / set-mode restart the supervisor)
- ❌ Plugin system
- ❌ TPM / HSM integration

## Important Files

- `main.go` - Entry point, minimal
- `cmd/root.go` - Command setup, config loading
- `cmd/update.go` - Core update logic
- `internal/profile/profile.go` - Platform detection
- `internal/crypto/device_crypto.go` - Encryption implementation
- `.goreleaser.yaml` - Release configuration
- `justfile` - Build automation (run `just --list`)

## Common Issues & Solutions

1. **Permission denied**: Config files need 600, secure files need 400
2. **Encryption fails**: Device ID might not be accessible
3. **UDM persistence**: Must use `/data/` paths
4. **Windows paths**: Use `filepath.Join()` for cross-platform

## Development Guidelines

1. **Keep it simple** - Resist feature creep
2. **Test on target** - Especially UDM constraints
3. **Document sparingly** - Code should be self-explanatory
4. **Fail fast** - Clear error messages, no retries
5. **Respect the user** - Quiet mode for cron, verbose for debugging

## Contact & Repository

- Repository: https://github.com/descoped/dddns
- Issues: https://github.com/descoped/dddns/issues
- Primary use case: Personal home networks with dynamic IPs