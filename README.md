# Descoped Dynamic DNS - dddns

A lightweight CLI tool for updating AWS Route53 DNS A records with dynamic IP addresses. Designed for resource-constrained devices like Ubiquiti Dream Machines.

## Quick Start

```bash
# Install
curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/latest/download/dddns-$(uname -s)-$(uname -m)
chmod +x /usr/local/bin/dddns

# Configure
dddns config init

# Update DNS
dddns update
```

## Features

- **Single binary** - No dependencies, <10MB
- **Low memory** - <20MB runtime usage
- **Secure** - Device-specific encrypted credentials
- **Cron-friendly** - Quiet mode, persistent caching
- **Cross-platform** - Linux, macOS, Windows, ARM64

## Documentation

User guides are in [docs/](docs/):
- [Installation](docs/INSTALLATION.md) - Platform-specific installation
- [Quick Start](docs/QUICK_START.md) - Get running in 5 minutes
- [Configuration](docs/CONFIGURATION.md) - Configuration options
- [Commands](docs/COMMANDS.md) - Command reference
- [UDM Guide](docs/UDM_GUIDE.md) - Ubiquiti Dream Machine setup
- [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues

## Development

### Prerequisites

- Go 1.22+
- Make

### Build

```bash
# Local build
make build

# Cross-compile for UDM
make build-udm

# Run tests
make test

# Install locally
sudo make install
```

### Project Structure

```
cmd/           # CLI commands and flags
internal/
├── config/    # Configuration management
├── crypto/    # Device-specific encryption
├── dns/       # Route53 client
├── profile/   # Platform detection and paths
└── version/   # Version information
```

### Core Logic

1. **IP Detection** - Check public IP via checkip.amazonaws.com
2. **Change Detection** - Compare with cached IP from last run
3. **DNS Update** - Update Route53 A record if IP changed
4. **Caching** - Persist new IP to prevent unnecessary API calls

### Release Process

Uses [GoReleaser](https://goreleaser.com/) with git tags:

```bash
git tag v1.0.0
git push origin v1.0.0
# GitHub Actions automatically builds and releases
```

### Testing

```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./tests/...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Platform Profiles

dddns automatically detects platform and adjusts file paths:

| Platform | Config | Cache | Notes |
|----------|--------|-------|-------|
| UDM/UDR | `/data/.dddns/` | Persistent across reboots |
| Linux | `~/.dddns/` | Standard user directory |
| macOS | `~/.dddns/` | Standard user directory |
| Windows | `%APPDATA%/dddns/` | Windows standard location |
| Docker | `/config/` | Container volume mount |

### Security

- **No hardcoded credentials** - Uses AWS profiles or encrypted config
- **Device-specific encryption** - AES-256-GCM with hardware-derived keys
- **Secure file permissions** - 600 for config, 400 for encrypted files
- **Memory wiping** - Sensitive data cleared after use

## License

MIT License - see [LICENSE](LICENSE)