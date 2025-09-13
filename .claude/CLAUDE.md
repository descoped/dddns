# CLAUDE.md

This file provides guidance to Claude Code when working with dddns.

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
1. Check public IP via checkip.amazonaws.com
2. Compare with cached IP from last run
3. Update Route53 A record if changed
4. Cache new IP and exit

### Project Structure
```
cmd/                      # CLI commands
├── root.go              # Main command setup, config loading
├── config.go            # Config init/check commands
├── ip.go                # IP display command
├── update.go            # Main update logic
├── verify.go            # DNS verification
└── secure.go            # Secure config management

internal/
├── config/              # Configuration management
│   ├── config.go        # YAML config handling
│   └── secure_config.go # Encrypted config support
├── crypto/              # Security layer
│   └── device_crypto.go # Hardware-based encryption
├── dns/                 # AWS integration
│   └── route53.go       # Route53 client
├── profile/             # Platform detection
│   └── profile.go       # OS/device-specific paths
├── commands/myip/       # IP utilities
│   └── myip.go          # Public IP detection
├── constants/           # Shared constants
│   └── permissions.go   # File permission constants
└── version/             # Version info
    └── version.go       # Build-time version injection
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
dddns ip
dddns verify

# Security
dddns secure enable       # Convert to encrypted config
dddns secure disable      # Revert to plain config
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
- Config loading and validation
- IP detection and proxy checking
- Encryption/decryption
- Platform detection
- Route53 mocking

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
- Secure credential storage
- Platform auto-detection
- Persistent caching
- Proxy/VPN detection
- GoReleaser integration
- Comprehensive documentation

## Do NOT Add Unless Explicitly Asked

- ❌ Metrics/monitoring/telemetry
- ❌ Web UI or REST API
- ❌ Multiple DNS providers (Route53 only)
- ❌ Daemon/service mode
- ❌ Complex retry logic
- ❌ Service discovery
- ❌ Container orchestration
- ❌ Database storage
- ❌ Configuration hot-reload
- ❌ Plugin system

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