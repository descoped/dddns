# Address Feedback on Issue/PR #$ARGUMENTS

Respond to comments on a dddns issue or PR systematically.

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

## Phase 1: Determine Type and Fetch Context

```bash
gh pr view $ARGUMENTS --repo descoped/dddns --json title,body,state,headRefName 2>/dev/null \
  || gh issue view $ARGUMENTS --repo descoped/dddns
```

## Phase 2: Ensure Correct Branch

**For PRs** — check out the PR branch:
```bash
gh pr checkout $ARGUMENTS --repo descoped/dddns
```

**For Issues** — check for an existing branch:
```bash
git branch --list "*issue-$ARGUMENTS*"
```
Check it out if found; otherwise feedback may be pre-implementation.

**Read task/design context** if they exist:
```bash
ls .claude/issues/issue-$ARGUMENTS/
```

## Phase 3: Gather Comments

**PRs**:
```bash
gh pr view $ARGUMENTS --repo descoped/dddns --json reviews
gh api repos/descoped/dddns/pulls/$ARGUMENTS/comments
```

**Issues**:
```bash
gh api repos/descoped/dddns/issues/$ARGUMENTS/comments
```

## Phase 4: Create Checklist

List every feedback item that needs addressing:
- [ ] Item 1 from reviewer
- [ ] Item 2 from reviewer

## Phase 5: Address Each Item

For each item:
1. Read and understand the feedback.
2. Read the relevant code.
3. Make the change.
4. Commit with a descriptive conventional-format message referencing the feedback:
   ```bash
   git commit -m "fix: address review - specific change description"
   ```

Group related fixes into one commit; keep unrelated fixes in separate commits.

## Phase 6: Run Quality Checks

```bash
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
```

If `scripts/` touched:
```bash
bash -n scripts/install-on-unifi-os.sh
shellcheck scripts/install-on-unifi-os.sh
```

## Phase 7: Push

```bash
git push
```

## Phase 8: Respond

**PRs**:
```bash
gh pr comment $ARGUMENTS --repo descoped/dddns --body "RESPONSE"
```

**Issues**:
```bash
gh issue comment $ARGUMENTS --repo descoped/dddns --body "RESPONSE"
```

## Response Templates

### PR Review Response

```markdown
## Addressed Review Feedback

Thanks for the review. Summary of changes:

### Changes Made

**1. [Feedback summary]**
- [What was changed]
- Commit: `abc1234`

### Discussion Points

> [Quote from reviewer]

[Response.]

### Not Addressed

- **[Item]**: [Reason — usually: out of scope, or conflicts with `.claude/CLAUDE.md` constraint, or requires a separate issue]
```

### Issue Update

```markdown
## Update on Issue #$ARGUMENTS

### Status: [In Progress | Blocked | Resolved]

### Progress

- [x] [Done]
- [ ] [Remaining]

### Questions / Blockers

### Next Steps
```

### Resolution Comment

```markdown
## Resolved

This issue has been resolved in PR #XX.

### Summary

[What was implemented.]

### Verified

- [x] go test -race ./... passes
- [x] golangci-lint clean
- [ ] On-device validation: [UDR/UDM/Pi or N/A]
```

## Handling Different Feedback Types

- **Critical** — must fix; one commit per fix where possible; explain what was done.
- **Suggestion** — consider carefully; implement if agreeable; if not, explain why (typically: out of scope, or violates `.claude/CLAUDE.md` minimalism).
- **Question** — answer in the response; make code changes if the answer reveals an issue.

## Rules

- Address every comment — fix or explain why not.
- Run area-specific tests plus the full suite before pushing.
- Push changes **before** posting the response.
- Be professional and specific; cite commit SHAs.
- Working directory: `.claude/` and `git` commands run from project root.
