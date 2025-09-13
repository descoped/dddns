# Changelog

All notable changes to dddns will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.1] - 2025-09-14

### 🐛 Bug Fixes
- Fixed invalid 'go' configuration option in renovate.json (should be 'golang')
- Fixed interactive prompts when piping install script through curl (uses `/dev/tty`)
- Fixed UniFi OS v4 detection for UDR and newer UDM devices
- Fixed download detection for GoReleaser tar.gz archive format
- Fixed wrapper scripts for proper interactive execution

### ✨ Features
- Added UniFi OS v4 support for UDR and newer UDM devices
- Added interactive prompts to install script for better user experience
- Improved cron job logging visibility (removed --quiet flag)

### 📚 Documentation
- Updated all install script references from `install-dddns-udm.sh` to `install.sh`
- Added comprehensive feature list to README
- Added clear DDNS problem statement to README introduction
- Added mermaid flow diagram showing dddns update process
- Added Route53 pricing information (~$0.50/month)
- Improved technical language for ISP DHCP lease description
- Added MIT license and contribution guidelines

### 🔧 Improvements
- Consolidated multiple UDM install scripts into single `install.sh`
- Added comprehensive test harness for install.sh functions
- Removed .mcp.json from tracking

### ⬆️ Dependencies
- Updated Go to 1.25
- Updated github.com/spf13/cobra to v1.10.1
- Updated github.com/spf13/viper to v1.21.0
- Updated GitHub Actions workflows
- Migrated and fixed Renovate configuration

### 📊 Statistics
- **Commits since v0.1.0**: 26
- **Files changed**: 13
- **Additions**: 471 lines
- **Deletions**: 226 lines

## [v0.1.0] - 2025-09-13

### 🎉 Initial Release

This is the first official release of dddns (Descoped Dynamic DNS), a lightweight CLI tool for updating AWS Route53 DNS A records with dynamic IP addresses. Originally created to replace a bash script on Ubiquiti Dream Machine routers, it has evolved into a comprehensive cross-platform solution.

### ✨ Features

#### Core Functionality
- **DNS Updates**: Automatic Route53 A record updates when public IP changes
- **IP Detection**: Reliable public IP detection via checkip.amazonaws.com
- **Smart Caching**: Persistent IP caching to minimize unnecessary API calls
- **Proxy Protection**: Built-in proxy/VPN detection to prevent incorrect updates
- **Dry Run Mode**: Test updates without making actual changes
- **Force Updates**: Override cache to force DNS record updates
- **Quiet Mode**: Suppress output for cron job compatibility

#### Platform Support
- **Ubiquiti Dream Machine** (UDM/UDR) - ARM64 with persistent storage
- **Linux** - AMD64, ARM64, and ARM architectures
- **macOS** - Intel (AMD64) and Apple Silicon (ARM64)
- **Windows** - AMD64 and ARM64 architectures
- **Docker** - Container deployment support

#### Security Features
- **Device-Specific Encryption**: AES-256-GCM encryption with hardware-derived keys
- **Secure Credential Storage**: Encrypted config files locked to specific hardware
- **AWS Profile Support**: Integration with AWS CLI credentials
- **File Permission Enforcement**: Automatic permission setting (600/400)
- **No Environment Variables**: Credentials stored securely in config files only
- **Memory Wiping**: Sensitive data cleared from memory after use

#### Commands
- `dddns update` - Update DNS record if IP changed
- `dddns config init` - Interactive configuration setup
- `dddns config check` - Validate configuration
- `dddns ip` - Display current public IP
- `dddns verify` - Check if DNS matches current IP
- `dddns secure enable` - Convert to encrypted config
- `dddns secure disable` - Revert to plain config
- `dddns secure test` - Test device encryption

#### Build & Deployment
- **Single Binary**: No dependencies, easy deployment
- **Cross-Platform Builds**: Automated builds for all major platforms
- **GoReleaser Integration**: Professional release automation
- **GitHub Actions CI/CD**: Automated testing and releases
- **UDM Install Script**: One-line installation for Ubiquiti devices
- **Persistent Boot Scripts**: Survives UDM firmware updates

### 🛠️ Technical Details

#### Architecture
- **Language**: Go 1.22+
- **Memory Usage**: <20MB runtime
- **Binary Size**: <10MB compressed
- **Dependencies**: Minimal (AWS SDK, Cobra, Viper)
- **Configuration**: YAML-based with schema validation

#### Platform Detection
- Automatic platform detection with tailored paths
- Platform-specific device ID extraction:
  - UDM: Serial number from `/proc/ubnthal/system.info`
  - Linux: MAC address from `/sys/class/net/eth0/address`
  - macOS: Hardware UUID via `system_profiler`
  - Windows: Machine GUID via `wmic`
  - Docker: Container ID from `/proc/self/cgroup`

#### File Locations
| Platform | Config Path | Cache Path |
|----------|------------|------------|
| UDM/UDR | `/data/.dddns/` | `/data/.dddns/last-ip.txt` |
| Linux | `~/.dddns/` | `~/.dddns/last-ip.txt` |
| macOS | `~/.dddns/` | `~/.dddns/last-ip.txt` |
| Windows | `%APPDATA%/dddns/` | `%APPDATA%/dddns/last-ip.txt` |
| Docker | `/config/` | `/config/last-ip.txt` |

### 📚 Documentation
- Comprehensive user guides in `docs/` directory
- Platform-specific installation instructions
- Quick start guide for 5-minute setup
- Troubleshooting guide with common issues
- UDM-specific deployment guide
- Developer-focused README

### 🔧 Development Infrastructure
- **CI/CD Pipeline**: GitHub Actions for testing and building
- **Version Management**: Git tags with ldflags injection
- **Dependency Management**: Renovate bot configuration
- **Code Quality**: Go vet, golangci-lint, unit tests
- **Release Automation**: GoReleaser configuration
- **Cross-Compilation**: Makefile with platform targets

### 📊 Statistics
- **Total Files**: 48
- **Lines of Code**: ~6,900
- **Platforms Supported**: 5 (UDM, Linux, macOS, Windows, Docker)
- **Binary Variants**: 9 (different OS/architecture combinations)
- **Test Coverage**: Core functionality covered
- **Commands**: 11 (including subcommands)

### 🏗️ Project Structure
```
cmd/                  # CLI commands and flags
internal/
├── config/          # Configuration management
├── crypto/          # Device-specific encryption
├── dns/             # Route53 client
├── profile/         # Platform detection
├── commands/myip/   # IP detection utilities
├── constants/       # Shared constants
└── version/         # Version information
docs/                # User documentation
scripts/             # Installation scripts
tests/               # Integration tests
```

### 🙏 Acknowledgments
- Built as a lightweight replacement for complex DDNS solutions
- Designed specifically for memory-constrained devices
- Inspired by the need for reliable home network DNS updates
- Created for the Ubiquiti Dream Machine community

### 📝 Notes
- First release focused on core functionality and stability
- Extensive testing on UDM7, Linux, and macOS platforms
- Production-ready for home network deployments
- Follows "Do As Little As Possible To Make It Work" philosophy

---

## Version History

- **v0.1.1** (2025-09-14) - Bug fixes, UniFi OS v4 support, documentation improvements
- **v0.1.0** (2025-09-13) - Initial release with core functionality

[Unreleased]: https://github.com/descoped/dddns/compare/v0.1.1...HEAD
[v0.1.1]: https://github.com/descoped/dddns/releases/tag/v0.1.1
[v0.1.0]: https://github.com/descoped/dddns/releases/tag/v0.1.0
