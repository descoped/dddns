# Release Management Process

This document describes the release management process for the `dddns` project.

## Overview

Releases are managed through git tags with automated binary building and GitHub Release creation via GoReleaser. The process follows semantic versioning (SemVer) with `v` prefixed tags.

## Release Process

### 1. Pre-Release Preparation

Before creating a release, ensure:

- [ ] All intended changes are merged into the `main` branch
- [ ] Tests are passing on the latest commit (`make test`)
- [ ] Build succeeds for all platforms (`make build-all`)
- [ ] Documentation is up to date
- [ ] CHANGELOG or release notes are prepared

### 2. Version Management

#### Semantic Versioning
Follow [SemVer](https://semver.org/) guidelines:
- **MAJOR** (`v1.0.0 → v2.0.0`): Breaking changes
- **MINOR** (`v1.0.0 → v1.1.0`): New features, backward compatible
- **PATCH** (`v1.0.0 → v1.0.1`): Bug fixes, backward compatible

### 3. Creating a Release

#### Option A: Command Line (Recommended)

```bash
# Ensure you're on main branch with latest changes
git checkout main
git pull origin main

# Create and push a new tag
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

This automatically triggers the GoReleaser workflow which:
- Builds binaries for all platforms
- Creates GitHub Release
- Uploads artifacts
- Generates changelog

#### Option B: GitHub UI

1. Go to the repository on GitHub
2. Click "Releases" → "Draft a new release"
3. Click "Choose a tag" → Type new tag (e.g., `v1.2.3`)
4. Select "Create new tag: v1.2.3 on publish"
5. Target: `main` branch
6. Add release notes
7. Click "Publish release"

### 4. Automated Workflow

The GoReleaser workflow (`.github/workflows/goreleaser.yml`) automatically:

1. **Validates** the release:
   - Checks tag format matches `v*` pattern
   - Extracts version for binary naming

2. **Runs quality checks**:
   - Executes `go test ./...`
   - Runs `go mod tidy`

3. **Builds binaries**:
   - Linux: amd64, arm64, arm
   - macOS: amd64 (Intel), arm64 (Apple Silicon)
   - Windows: amd64, arm64
   - Special UDM build (linux/arm64)

4. **Creates release**:
   - Generates changelog from commit history
   - Creates GitHub Release
   - Uploads binaries and checksums
   - Archives with LICENSE and docs

### 5. Post-Release Verification

After the workflow completes:

- [ ] Check the [Releases page](https://github.com/descoped/dddns/releases) for new release
- [ ] Download and test a binary for your platform
- [ ] Verify checksums: `sha256sum -c checksums.txt`
- [ ] Test installation scripts work correctly
- [ ] Update any deployment documentation if needed

## Release Notes Template

Use this template for release descriptions:

```markdown
## What's Changed

### 🚀 New Features
- Feature description (#PR)

### 🐛 Bug Fixes
- Bug fix description (#PR)

### 🔒 Security
- Security updates

### 📚 Documentation
- Documentation improvements

### 🔧 Internal Changes
- Performance improvements
- Refactoring

### 📦 Dependencies
- Update aws-sdk-go-v2 to v1.x.x
- Update other dependencies

### ⚠️ Breaking Changes
- Description of breaking changes (if any)

**Full Changelog**: https://github.com/descoped/dddns/compare/v1.2.2...v1.2.3

## Installation

### Ubiquiti Dream Machine
```bash
curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh | bash
```

### Linux/macOS
```bash
curl -L -o /usr/local/bin/dddns \
  https://github.com/descoped/dddns/releases/download/v1.2.3/dddns-$(uname -s)-$(uname -m)
chmod +x /usr/local/bin/dddns
```
```

## Platform-Specific Binaries

Each release includes binaries for:

| Platform | Architecture | Binary Name |
|----------|-------------|-------------|
| Linux | AMD64 | `dddns-linux-amd64` |
| Linux | ARM64 | `dddns-linux-arm64` |
| Linux | ARM | `dddns-linux-arm` |
| macOS | Intel | `dddns-darwin-amd64` |
| macOS | Apple Silicon | `dddns-darwin-arm64` |
| Windows | AMD64 | `dddns-windows-amd64.exe` |
| Windows | ARM64 | `dddns-windows-arm64.exe` |

### Special Builds
- **UDM/UDR**: Use `dddns-linux-arm64`
- **Raspberry Pi**: Use `dddns-linux-arm64` or `dddns-linux-arm`

## Troubleshooting

### Common Issues

#### Tag Already Exists
```
fatal: tag 'v1.2.3' already exists
```

**Solution**: Delete the tag and recreate:
```bash
git tag -d v1.2.3
git push origin --delete v1.2.3
# Then create new tag
```

#### GoReleaser Fails
Check the [Actions tab](https://github.com/descoped/dddns/actions) for detailed logs.

Common causes:
- Test failures - Fix tests before tagging
- Invalid `.goreleaser.yaml` - Validate configuration
- Network issues - Retry the workflow

#### Missing Binaries
If certain platform binaries are missing:
1. Check `.goreleaser.yaml` build configuration
2. Verify platform is not in ignore list
3. Check for platform-specific build errors

### Manual Release Recovery

If automated release fails:

1. **Check workflow logs** for specific errors
2. **Fix the issue** in code
3. **Delete the failed release and tag** from GitHub
4. **Create a new tag** with incremented version
5. **Push the new tag** to trigger workflow

### Emergency Hotfix Process

For critical security fixes:

1. Create hotfix branch from latest release tag:
   ```bash
   git checkout -b hotfix/v1.2.4 v1.2.3
   ```

2. Apply fix and commit:
   ```bash
   git add .
   git commit -m "fix: critical security issue"
   ```

3. Tag and release:
   ```bash
   git tag -a v1.2.4 -m "Security hotfix v1.2.4"
   git push origin v1.2.4
   ```

4. Merge back to main:
   ```bash
   git checkout main
   git merge hotfix/v1.2.4
   git push origin main
   ```

## Security Considerations

- **Signed Commits**: Consider GPG signing release tags
  ```bash
  git tag -s v1.2.3 -m "Release v1.2.3"
  ```

- **Checksums**: Always verify SHA256 checksums for downloaded binaries
  ```bash
  sha256sum -c checksums.txt
  ```

- **Binary Verification**: Binaries are built in GitHub Actions with full build logs

- **Minimal Permissions**: Workflow uses minimal required permissions

## Version Injection

Version information is injected at build time via ldflags:

```go
// internal/version/version.go
var Version = "dev"      // Set to tag version
var BuildDate = "unknown" // Set to build timestamp
var Commit = "none"      // Set to git commit hash
```

Users can verify version:
```bash
dddns --version
# Output: dddns version v1.2.3 (built 2024-01-15T10:30:00Z, commit abc123)
```

## Monitoring

Monitor releases through:
- GitHub Actions workflow status
- GitHub release download counts
- Issue reports for version-specific problems
- UDM community feedback

## Release Cadence

- **Patch releases**: As needed for bug fixes
- **Minor releases**: Monthly for new features (if any)
- **Major releases**: Only for breaking changes

## Rollback Procedure

If a release has critical issues:

1. Mark release as pre-release on GitHub
2. Point users to previous stable version
3. Fix issues and release new patch version
4. Consider yanking if security-critical

## Contact

For release-related issues:
- Open an issue: https://github.com/descoped/dddns/issues
- Tag with `release` label