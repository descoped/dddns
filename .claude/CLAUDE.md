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
Two run modes, selected at install time (exclusive):

**Cron mode** (polling):
1. Resolve current public IP via `cfg.IPSource` — `remote` calls `checkip.amazonaws.com`, `local` reads the WAN interface, `auto` picks based on platform.
2. Compare with cached IP from last run.
3. Update Route53 A record if changed.
4. Cache new IP and exit.

**Serve mode** (event-driven, UniFi-only):
1. `dddns serve` listens on `127.0.0.1:53353` for dyndns v2 requests from UniFi's built-in `inadyn`.
2. Per request: CIDR check → constant-time Basic Auth → hostname match → read local WAN IP (never trust `myip` query param) → Route53 UPSERT → audit log.
3. Supervised by systemd; restart-on-failure.

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
```

## Key Features Implemented

### Security
- ✅ Device-specific AES-256-GCM encryption
- ✅ Hardware ID derivation (MAC, UUID, serial)
- ✅ Secure file permissions (600/400)
- ✅ No environment variable credentials
- ✅ AWS profile support via ~/.aws/credentials

### Platform Support
- ✅ UDM/UDR (ARM64, persistent /data storage)
- ✅ Linux (AMD64/ARM64/ARM)
- ✅ macOS (Intel/Apple Silicon)
- ✅ Windows (AMD64/ARM64)
- ✅ Docker containers

### Commands
```bash
# Core commands
dddns update [--dry-run] [--force] [--quiet]
dddns config init
dddns config check
dddns config set-mode {cron|serve}   # Switch run mode; rewrites boot script
dddns config rotate-secret [--init]  # Rotate serve-mode shared secret
dddns ip
dddns verify

# Serve mode (UniFi bridge)
dddns serve                          # Start listener (blocks)
dddns serve status                   # Show last request / auth outcome
dddns serve test                     # Loopback Basic-Auth'd test request

# Security
dddns secure enable       # Convert to encrypted config
dddns secure test         # Test encryption
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
make build           # Current platform
make build-udm       # UDM ARM64
make build-all       # All platforms
make test            # Run tests
make install         # Install to /usr/local/bin
```

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

### Unit Tests
- Config loading and validation (`ServerConfig` fail-closed checks)
- IP detection and `ValidatePublicIP` stdlib validator
- Encryption/decryption (`EncryptString`/`DecryptString` round-trip + GCM tamper)
- Platform detection
- Route53 mocking
- Serve-mode handler (CIDR / auth / lockout / audit / status — all `httptest`-backed, no AWS)
- Boot-script generation per mode

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

### UDM Installation
```bash
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-dddns-udm.sh | bash
```

### Cron Setup
```bash
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
```

## Current Status

✅ **Completed**
- Core DNS update functionality
- Cross-platform support
- Secure credential storage (AES-256-GCM, device-derived key)
- Platform auto-detection
- Persistent caching
- UniFi serve mode (event-driven via inadyn push, systemd-supervised)
- Scoped Route53 IAM policy (record-level UPSERT via condition keys)
- Secret rotation (`dddns config rotate-secret`)
- Mode switching (`dddns config set-mode {cron|serve}`)
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
- `Makefile` - Build automation

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