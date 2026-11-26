# Releasing dddns

Releases are git tags (`v<major>.<minor>.<patch>`). Pushing a tag runs
`.github/workflows/goreleaser.yml`, which builds every platform and publishes the GitHub release.

## Make a release

```bash
just release 0.3.4             # checks, shows the notes, asks once, then releases
just release 0.3.4 --dry-run   # only checks and shows the notes
```

With Claude Code, saying "release" does the same, and also picks the version and writes the notes
(`.claude/commands/release.md`).

`just release` (`scripts/release.sh`) stops, with nothing changed, unless:

- you are on `main`, in step with GitHub, with no local changes other than to `CHANGELOG.md`;
- the version is newer than the last tag;
- the last CI run on `main` passed, and `just check` (fmt, vet, race tests, lint) passes;
- `CHANGELOG.md` has notes under `## [Unreleased]`.

It then moves those notes under `## [v0.3.4] - <date>`, commits, tags, pushes, and waits for the release
build to finish.

## Versions

[Semantic versioning](https://semver.org/):

- **patch** (0.3.3 → 0.3.4): fixes and dependency updates;
- **minor** (0.3.3 → 0.4.0): new features, nothing breaks;
- **major**: a breaking change, such as a removed command, flag or config key, or a config file that no
  longer loads.

## Release notes

The GitHub release text is the version's section of `CHANGELOG.md`, between the header and the install
footer from `.goreleaser.yaml`. Write for people who run dddns: what changed for them, one bullet each,
under the headings already used in the file.

## What GitHub builds

GoReleaser builds Linux (amd64, arm64, armv7), macOS (Intel, Apple Silicon) and Windows (amd64, arm64)
archives, `.deb`, `.rpm` and `.apk` packages for Linux, and `checksums.txt`. The version, build date and
commit are set through ldflags (`internal/version/`); `dddns --version` shows them.

Check a download against the checksums:

```bash
sha256sum -c checksums.txt --ignore-missing
```

## If something goes wrong

- **`just release` stops:** it says why and nothing has changed. Fix what it names and run it again.
- **The release build fails after the tag is pushed:** see the run in the
  [Actions tab](https://github.com/descoped/dddns/actions). If the cause was outside the code (a network
  error, a GitHub outage), re-run it: `gh run rerun <run-id>`. If the code or config was wrong, fix it on
  `main` and release the next patch version; do not move a published tag.
- **A release is broken:** mark it as a pre-release on GitHub so `latest` points at the previous one, then
  release a fix.
