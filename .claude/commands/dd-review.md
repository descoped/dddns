# Review PR #$ARGUMENTS

Review a dddns pull request with a structured checklist.

## Configuration

```
Repo: descoped/dddns
Main branch: main
```

### Workspace Areas

| Area | Path | Test command |
|------|------|--------------|
| `core` | `internal/{updater,dns,commands/myip}` | `go test -race ./internal/updater/... ./internal/dns/... ./internal/commands/...` |
| `cli` | `cmd/` | `go test -race ./cmd/...` |
| `server` | `internal/{server,bootscript}`, `cmd/serve.go`, `cmd/config_*.go` | `go test -race ./internal/server/... ./internal/bootscript/... ./cmd/...` |
| `security` | `internal/{config,crypto}` | `go test -race ./internal/config/... ./internal/crypto/... ./internal/server/...` |
| `platform` | `internal/{profile,wanip}` | `go test -race ./internal/profile/... ./internal/wanip/...` |
| `installer` | `scripts/install-on-unifi-os.sh` | `bash -n scripts/install-on-unifi-os.sh && shellcheck scripts/install-on-unifi-os.sh` |

## Phase 1: Verify Branch

```bash
CURRENT_BRANCH=$(git branch --show-current)
PR_BRANCH=$(gh pr view $ARGUMENTS --repo descoped/dddns --json headRefName -q .headRefName)
if [ "$CURRENT_BRANCH" != "$PR_BRANCH" ]; then
  echo "Switching from $CURRENT_BRANCH to $PR_BRANCH..."
  git fetch origin
  git checkout "$PR_BRANCH"
fi
```

## Phase 2: Fetch PR Details

```bash
gh pr view $ARGUMENTS --repo descoped/dddns \
  --json title,body,headRefName,baseRefName,additions,deletions,changedFiles,commits,files,reviews,labels
```

If previous reviews exist, read them and track which feedback was addressed.

## Phase 3: Get Full Diff

```bash
gh pr diff $ARGUMENTS --repo descoped/dddns
```

## Phase 4: Read Context and Changed Files

```bash
ls .claude/issues/issue-*/
```

Read `task.md` and `design.md` if they exist. Then read each changed file with the Read tool for full context — don't rely on the diff alone.

## Phase 5: Run Checks Locally

```bash
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
```

If the PR touches `scripts/`:
```bash
bash -n scripts/install-on-unifi-os.sh
shellcheck scripts/install-on-unifi-os.sh
```

Run the per-area test command (from the Workspace Areas table) for every area the PR touches.

## Phase 6: Review Checklist

### Conventions
- [ ] Commits follow conventional format (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`)
- [ ] No AI attribution in commits
- [ ] Task file exists in `.claude/issues/` (if issue-based PR)
- [ ] PR description follows the template in `.github/PULL_REQUEST_TEMPLATE.md`
- [ ] `fixes #X` links the issue correctly

### Code Quality (Go)
- [ ] No ignored errors — discarded return values must be explicit (`_ = call()`) with a reason
- [ ] `context.Context` is propagated on every external call (AWS, HTTP)
- [ ] Goroutine lifecycles are bounded — join via channel or waitgroup; nothing outlives `t.TempDir()` in tests
- [ ] No `panic` in request-handling paths (handler recovers to `911`; outside that, panics are a bug)
- [ ] No secrets in code, committed configs, or test fixtures
- [ ] Fail-closed validation for new config fields
- [ ] File permissions respected (600 for plaintext, 400 for `.secure`)
- [ ] `filepath.Dir` / `filepath.Join` used for path manipulation (not string slicing)

### Architecture (dddns-specific)
- [ ] Change respects `.claude/CLAUDE.md` "Do NOT Add" list (no new daemon modes, retry logic, DB storage, plugin system, hot-reload, etc., unless explicitly authorized)
- [ ] Memory footprint respected — UDM target <20 MB resident. Flag any new long-lived allocations or heavy dependencies.
- [ ] No new Go module dependencies unless justified in the PR description
- [ ] For `server`/`security` changes — L1–L6 threat-model constraints from `ai_docs/5_unifi-ddns-bridge.md` §3 are not weakened
- [ ] For `core` changes to the update flow — `cmd/update.go` remains a thin shim over `internal/updater`
- [ ] For multi-provider work — change is per `ai_docs/0_provider-architecture.md` (HTTP-only, extends `updater.DNSClient`)

### Testing
- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run` clean
- [ ] `go vet ./...` clean
- [ ] Per-area test commands (from table) pass for every touched area
- [ ] New tests cover new public behavior and regression-inducing edge cases
- [ ] `httptest` used for external API mocking — no live AWS/HTTP calls in CI
- [ ] If `server`/`platform`/`installer` touched: on-device validation plan stated in PR description

### Documentation
- [ ] Code comments only where the WHY is non-obvious (no narration of WHAT)
- [ ] `docs/` updated if user-facing behavior changed
- [ ] `ai_docs/` updated if architectural direction changed, including Status / Confidence / Last reviewed fields
- [ ] PR description is complete — Summary, fixes #, Changes, Testing, Acceptance Criteria

## Phase 7: Submit Review

**Self-review check** — GitHub does not allow approving your own PR:
```bash
PR_AUTHOR=$(gh pr view $ARGUMENTS --repo descoped/dddns --json author -q .author.login)
CURRENT_USER=$(gh api user -q .login)
```

- Own PR → `--comment`
- Others' PR → `--approve` or `--request-changes`

```bash
# Others' PR
gh pr review $ARGUMENTS --repo descoped/dddns --approve          --body "REVIEW"
gh pr review $ARGUMENTS --repo descoped/dddns --request-changes  --body "REVIEW"

# Own PR (self-review)
gh pr review $ARGUMENTS --repo descoped/dddns --comment          --body "REVIEW"
```

## Review Format

### Initial Review

```markdown
## Review: [Approve | Needs Changes | Comment]

[1-2 sentence overall assessment.]

### Critical Issues

**1. [Title]** (`path/to/file:LINE`)

[Problem description.]

\```
// Suggested fix
\```

### Suggestions (non-blocking)

- [Point 1]
- [Point 2]

### What Looks Good

- [Positive 1]

### Questions

1. [Clarifying question]

### Checklist

- [x] Conventions followed
- [x] Build passes
- [x] Code quality acceptable
- [ ] [Any failed items]
```

### Follow-up Review

```markdown
## Follow-up Review: [Approve | Needs Changes]

[Assessment of changes since last review.]

### Previous Feedback Status

| Feedback | Status |
|----------|--------|
| [Summary] | Fixed / Not addressed / Explained |

### New Issues (if any)

### Ready to Merge

[Yes/No — brief reason.]
```

## Severity Levels

- **Critical** — must fix before merge (bugs, security, correctness)
- **Suggestion** — non-blocking (style, minor improvements)
- **Question** — clarification; may or may not require changes

## Rules

- Verify correct PR branch before reviewing.
- Read actual files, not just the diff.
- Run the area-specific test command for every touched area, plus the full suite.
- Distinguish critical from suggestion; be specific with file:line references.
- Working directory: `.claude/` and `git` commands run from project root.
