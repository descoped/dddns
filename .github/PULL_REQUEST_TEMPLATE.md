## Summary

<!-- Brief description of what was changed and why (2-3 sentences) -->

## Closes Issue

<!-- Use "fixes #X" to auto-close the issue on merge -->
fixes #

## Task File

<!-- Task file stays in .claude/issues/ until PR is merged, then archived to history -->
Task: `.claude/issues/issue-X/task.md`

## Area

- [ ] `core` — update flow, Route53 client, IP detection
- [ ] `cli` — CLI commands, flags, user-facing UX
- [ ] `server` — serve-mode HTTP handler
- [ ] `security` — config encryption, auth, audit log, IAM
- [ ] `platform` — UDM/Pi/Linux/macOS/Windows specifics
- [ ] `installer` — install scripts, GoReleaser, Makefile
- [ ] `docs` — documentation only

## Changes

-
-
-

## Testing

- [ ] `go fmt ./...` clean
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean
- [ ] `go test -race ./...` passes
- [ ] `shellcheck` clean (if shell scripts changed)
- [ ] On-device validation (if `server`/`platform`/`installer` touched — note which device)

## Acceptance Criteria

<!-- Copy from task file -->
- [ ]
- [ ]

## Checklist

- [ ] Commits follow conventional format (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`)
- [ ] Task file exists in `.claude/issues/` (moves to history post-merge)
- [ ] No AI attribution in commits
- [ ] No backward-compat shims for removed behavior (per project CLAUDE.md)
- [ ] Memory footprint respected (UDM target: <20 MB resident)
