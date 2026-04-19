# Changelog

All notable changes to dddns will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

(empty)

## [v0.3.2] - 2026-04-19

### ✨ Features
- **Lambda dry-run** (`?dry-run=true` query param) — runs the full auth + hostname-match pipeline but skips Route53 UPSERT; returns `good <ip> (dry-run)`. Operator-facing probe for verifying an endpoint without mutation. Auth and hostname checks are still enforced (no information leak on dry-run).

### 🔧 Changed
- AWS provider v6 compat — `deploy/aws-lambda/tofu/iam.tf` switched `data.aws_region.current.name` → `.region`. Removes 4 deprecation warnings from `tofu plan` after Renovate PR #41 bumped the provider to v6.
- Renovate PR #41: AWS provider bumped to v6.

### 🧪 Tests
- Short-mile coverage pass: `internal/config/defaults_test.go`, `cmd/update_test.go`, `cmd/serve_test.go` — 67.5% → 69.7% total. Covers `UpdateIntervalOrDefault` / `UpdateTimeoutOrDefault` matrix, `SavePlaintext` round-trip + perm enforcement, `runUpdate` RFC1918 rejection + Validate propagation, `runServeTest` no-server-block.
- 4 new Lambda handler tests: dry-run skips Route53, still enforces auth, still matches hostname, `isDryRun` truthy/falsy matrix.

### 🔧 CI
- **Codecov wiring hardened**: `CODECOV_TOKEN` passed via repo secret (required now that `main` is branch-protected); `fail_ci_if_error: true` on the upload step so silent drops are visible instead of leaving the badge stale.
- `codecov.yml` tuned: status checks `informational: true` (Codecov reports PR impact but doesn't block the merge button); `range: 50...80` so the badge gradient shows yellow at 66%; `require_ci_to_pass: true` suppresses PR comments when CI itself failed.

### 📚 Documentation
- `deploy/aws-lambda/README.md` — **Cost attribution** section. Documents the built-in `app=dddns` / `hostname=<...>` / `module=deploy/aws-lambda` tags, the out-of-band one-liner for tagging the pre-existing Route53 zone (which the module deliberately doesn't manage), and the `aws ce update-cost-allocation-tags-status` activation step (with the AWS-Organizations management-account caveat).

## [v0.3.1] - 2026-04-18

### 🧪 Tests
Test-coverage lift from ~54% to ~67.5%, concentrated in code that changes frequently. Philosophy: each test catches an invariant, security boundary, distinct error path, non-obvious branch, or regression. No test-ware added.

- `internal/verify` 0% → 92.9% — `Run` orchestration via injectable hooks; 4 distinct failure paths (IP mismatch, DNS mismatch, zero resolvers, per-resolver timeout)
- `internal/config` → 80%+ — 0400 perm enforcement, AES-GCM tamper detection (both vaults), secure round-trip, migration wipes plaintext
- `cmd/` — `runConfigCheck` (5 paths), `runConfigInit` (0600 + --force), `runServeStatus` (present/absent/malformed), `formatVerifyReport` (5 rendering shapes)
- `internal/dns` — `parseAWSError` 30% → 100% across three XML shapes + oversized truncation
- `internal/crypto` — `deviceIDFallback` 0% → 100% (USER, USERNAME, bare hostname, deterministic)
- `internal/updater` — GetCurrentIP failure doesn't abort UPSERT; UpdateIP error propagates; DryRun with cache
- `internal/server` — `StatusWriter.Write` failure paths (parent-missing, read-only parent, target-is-dir, no temp leak)

### 🔧 Changed
- `update_interval` + `update_timeout` now YAML-configurable (`config.UpdateIntervalOrDefault`, `config.UpdateTimeoutOrDefault`) — raise for slow networks
- `cmd/root.go` — `fatalf()` extracted (8 call sites)
- `.golangci.yml` v2 — `errcheck` exclusions for `fmt.F*` writes
- Installer `--disable` respected on upgrade (no mode regeneration unless explicitly requested)
- Installer `invocation_hint` — multi-word suffixes preserved (`$*` instead of `$1`)

### 📚 Documentation
- Coverage plan at `ai_docs/20260418T2259Z_test-coverage-plan.md`
- `llm-skills/session-snapshot/` skill added (save/restore around `/compact`)

## [v0.3.0] - 2026-04-18

### ✨ Features — AWS Lambda deployment form
Alternative to cron/serve mode: run dddns as a Lambda behind API Gateway HTTP API. The right choice when UniFi's built-in `inadyn` is the push source — `inadyn`'s `-b eth4` binding cannot reach a loopback listener, but it can reach a public HTTPS endpoint.

- `deploy/aws-lambda/` — handler, SSM client (hand-rolled, reuses `internal/dns` SigV4), dyndns v2 protocol, sourceIp-authoritative (ignores `myip` query param)
- `deploy/aws-lambda/tofu/` — OpenTofu module provisioning Lambda + API Gateway + IAM + SSM + CloudWatch. 13 resources, all variables — no hardcoded account/region/zone
- IAM scoped to one zone + one record + UPSERT-only + A-only + one SSM parameter + KMS `aws/ssm` with `kms:ViaService` condition
- `deploy/aws-lambda/scripts/rotate-secret.sh` — openssl rand + ssm put-parameter
- `just build-aws-lambda` — Linux arm64 zip for `provided.al2023`
- Cold-start < 150 ms, 128 MB memory, ~28 MB actual usage

### 🔧 Changed
- Hand-rolled SigV4 signer extended with STS session token support (`X-Amz-Security-Token` header) — shared between Route53 and SSM

## [v0.2.1] - 2026-04-18

### ✨ Features
- **Cron-mode output routed through `logger -t dddns` to systemd-journald** — rotation-free, no more flat `/var/log/dddns.log`
- `dddns update --verbose` — per-step diagnostics for cron investigation (overrides `--quiet`)
- Installer `--probe` reports firmware version + upstream reachability
- `bootscript.Params.UpdateInterval` — caller populates; `DefaultUnifiParams` no longer bakes schedule
- `Environment=GOMEMLIMIT=16MiB` in the serve-mode systemd unit (UDM/UDR memory budget)

### 🧪 Tests
- `internal/server/handler_bench_test.go` — BenchmarkHandler_HappyPath (alloc tracking across releases)

### 📚 Documentation
- "Where Do My Logs Live?" section in `docs/troubleshooting.md`

## [v0.2.0] - 2026-04-18

### ✨ Features
- **Serve mode** (`dddns serve`) — event-driven via dyndns v2 push; 127.0.0.1 listener; systemd-supervised
  - `dddns serve status` / `dddns serve test` for diagnostics
  - CIDR allowlist + constant-time Basic Auth + sliding-window lockout
  - JSONL audit log with rotation
  - **Experimental on UniFi Dream devices** — UniFi's `inadyn -b eth4` binding cannot reach the loopback listener; works on Pi/Linux/Docker with a same-host DDNS client
- `dddns config set-mode {cron|serve}` — switch run mode; rewrites boot script
- `dddns config rotate-secret` — rotate the serve-mode shared secret
- Installer hardened for RC iteration — `--force`, `--uninstall`, `--rollback`, `--version`, state snapshots, smoke tests
- Installer adds bash-native ELF inspector fallback in `--probe`
- `internal/wanip` — fall back to interface scan when no default route is configured

### 🔧 Changed
- **Retired AWS SDK and viper** — stdlib-only dependencies (significant footprint reduction on UDM/UDR)
- Migrated from archived `gopkg.in/yaml.v3` to `go.yaml.in/yaml/v3`
- Go toolchain aligned to 1.26 across go.mod + CI
- Installer log output routed to stderr
- Sensible rollback hint when installer invoked via curl-pipe

### 🔒 Security
- Hardening pass from v0.2.0 security review (see `ai_docs/20260418T1659Z_security-review.md`)
- Privacy-safe fixtures (RFC 5737 / RFC 2606) throughout tests

### 📚 Documentation
- `ai_docs/5_unifi-ddns-bridge.md` — serve-mode threat model (L1–L6)
- Community release plan, security review, provider-architecture design docs

### 🧪 Tests
- Serve-mode handler (CIDR / auth / lockout / audit / status — `httptest`-backed, no AWS)
- 248 tests across 16 packages

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

- **v0.3.2** (2026-04-19) — Lambda dry-run + coverage lift to 69.7% + AWS v6 + Codecov hardening
- **v0.3.1** (2026-04-18) — Test coverage 54% → 67.5%; configurable `update_interval`/`update_timeout`; `fatalf` extraction
- **v0.3.0** (2026-04-18) — AWS Lambda deployment form (API Gateway + Route53 + SSM + OpenTofu module)
- **v0.2.1** (2026-04-18) — Cron output → systemd-journald; `--verbose`; installer `--probe`; GOMEMLIMIT on serve
- **v0.2.0** (2026-04-18) — Serve mode; AWS SDK + viper retired (stdlib-only); Go 1.26; installer hardening
- **v0.1.1** (2025-09-14) — Bug fixes, UniFi OS v4 support, documentation improvements
- **v0.1.0** (2025-09-13) — Initial release with core functionality

[Unreleased]: https://github.com/descoped/dddns/compare/v0.3.2...HEAD
[v0.3.2]: https://github.com/descoped/dddns/releases/tag/v0.3.2
[v0.3.1]: https://github.com/descoped/dddns/releases/tag/v0.3.1
[v0.3.0]: https://github.com/descoped/dddns/releases/tag/v0.3.0
[v0.2.1]: https://github.com/descoped/dddns/releases/tag/v0.2.1
[v0.2.0]: https://github.com/descoped/dddns/releases/tag/v0.2.0
[v0.1.1]: https://github.com/descoped/dddns/releases/tag/v0.1.1
[v0.1.0]: https://github.com/descoped/dddns/releases/tag/v0.1.0
