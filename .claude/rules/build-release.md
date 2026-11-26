# Build, Test & Release

Applies to: whole project.

## Local development

```bash
just build             # Current platform
just dev               # Race-detector build
just test              # Run tests
just install           # Install to /usr/local/bin
just build-aws-lambda  # Linux arm64 zip for AWS Lambda deployment
just --list            # Full recipe list
```

One-off cross-build: `GOOS=... GOARCH=... go build .`

## Release process

- **When the user says "release"**, follow `.claude/commands/release.md`: pick the version, write the
  notes, run `just release <x.y.z> --yes`. Saying "release" is the permission to commit, tag and push.
- `just release <x.y.z>` (`scripts/release.sh`) checks main is clean and in step with GitHub, CI passed, the
  version is newer and `just check` passes; moves the `[Unreleased]` notes in `CHANGELOG.md` under the
  version; commits, tags, pushes; and waits for the release build. `--dry-run` only checks.
- The GitHub release text is the version's section of `CHANGELOG.md` (`goreleaser.yml`), plus the header
  and install footer from `.goreleaser.yaml`.
- This repo is public: release notes, commits and docs never mention where or how anyone deploys it.
- GoReleaser + git tags; automated via `.github/workflows/goreleaser.yml`.
- Cross-platform binaries: UDM arm64, Linux amd64/arm/arm64, macOS Intel + Apple Silicon, Windows amd64/arm64.
- SHA-256 release verification.
- Version injection via ldflags — see `internal/version/`.

## Testing

**Unit tests** (httptest / mocked; no network, no AWS):

- Config loading and validation (`ServerConfig` fail-closed checks)
- IP detection and `ValidatePublicIP` stdlib validator
- Encryption/decryption (round-trip + GCM tamper)
- Platform detection
- Route53 client + SigV4 signer (AWS reference-vector tests, with + without session token)
- Serve-mode handler (CIDR / auth / lockout / audit / status)
- Serve-mode memory-leak regression (`BenchmarkHandler_HappyPath`)
- Lambda handler (auth / hostname / sourceIp / SSM cache)
- Boot-script generation per mode (including journald pipe for cron)

**Integration tests** — real AWS calls when credentials available; file system operations; cross-platform compatibility.

## CI/CD

- `.github/workflows/ci.yml` — test, lint, build
- `.github/workflows/goreleaser.yml` — release automation
- `.github/renovate.json` — dependency updates

Quality gate: `go vet`, `golangci-lint run`, full `go test -race ./...`, multi-platform build verification.
