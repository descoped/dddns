# Start Work on Issue #$ARGUMENTS

Pick up a GitHub issue and run the full workflow: branch, design.md, task.md, implementation, PR, review, merge, archive.

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
| `docs` | `docs/`, `ai_docs/`, `README.md` | (no automated checks) |

## Phase 1: Fetch and Analyze Issue

```bash
gh issue view $ARGUMENTS --repo descoped/dddns
ls -la .claude/issues/issue-$ARGUMENTS/ 2>/dev/null || echo "No existing folder"
```

If `design.md` already exists, read it for context.

### Read ALL referenced documents

Scan the issue body for every referenced document — `ai_docs/*.md`, `docs/*.md`, linked issues/PRs, external URLs. **Read all of them before proceeding.** Also read the existing source in the areas the issue will touch.

**Determine scope from the issue** — dependency order for dddns:

1. `internal/{crypto,profile,wanip}` — leaf dependencies; touch only when the issue requires it.
2. `internal/{config,bootscript}` — depends on crypto/profile.
3. `internal/{dns,commands/myip}` — external integrations.
4. `internal/{updater,server}` — orchestration.
5. `cmd/` — CLI wiring, user-facing.
6. `docs/`, `ai_docs/`, `scripts/install-on-unifi-os.sh` — reflect completed code last.

## Phase 2: Setup

1. **Assign the issue**:
   ```bash
   gh issue edit $ARGUMENTS --repo descoped/dddns --add-assignee @me
   ```

2. **Ensure on latest main**:
   ```bash
   git checkout main && git pull origin main
   ```

3. **Create branch** (infer type from issue):
   ```bash
   git checkout -b feature/issue-$ARGUMENTS-short-description   # feature
   git checkout -b fix/issue-$ARGUMENTS-short-description       # bug
   git checkout -b docs/issue-$ARGUMENTS-short-description      # docs
   git checkout -b refactor/issue-$ARGUMENTS-short-description  # refactor
   ```

4. **Ensure issue folder exists**:
   ```bash
   mkdir -p .claude/issues/issue-$ARGUMENTS
   ```

5. **Create `design.md`** at `.claude/issues/issue-$ARGUMENTS/design.md` — **MANDATORY**.

   Fully self-contained. Copy specifications verbatim from `ai_docs/` and other sources. Never reference external documents by path — the content IS the spec.

   Required sections:
   ```markdown
   # Issue #N: [Title]

   **Status:** In progress
   **Branch:** <branch name>

   ## Specification
   [Copy ALL relevant content verbatim — specs, constraints, IAM policies, config schemas, etc.]

   ## Analysis
   - [What the issue requires]
   - [Existing code affected]
   - [Constraints and edge cases — memory footprint, UDM compatibility, fail-closed defaults]

   ## Design Decisions
   - [Approach chosen and why]
   - [Key types, functions, config keys to create or modify]

   ## Dependencies
   - [Other issues or PRs]
   - [External packages — check whether the addition crosses the minimal-footprint goal]
   ```

6. **Create `task.md`** at `.claude/issues/issue-$ARGUMENTS/task.md` — **MANDATORY**.

   ```markdown
   # Issue #N: [Title]

   ## Tasks

   - [ ] [Specific, actionable step]
   - [ ] ...

   ## Files to Create or Modify

   - `path/to/file` — [what changes]

   ## Acceptance Criteria

   - [ ] [Observable behavior 1]

   ## Progress Log

   [Updated during implementation]
   ```

7. **Inform the user** setup is complete — summarize design.md and list the tasks — then begin implementation.

## Phase 3: Implementation

Work through `task.md` systematically. Check off each task as done. Update the Progress Log with notes, decisions, and blockers.

Per-area during implementation, run the area's test command from the table above. For cross-area work, follow the dependency order in Phase 1.

**No backward compatibility shims** — per `.claude/CLAUDE.md`, remove old paths rather than dual-wiring. No `--deprecated` flags, no renamed helpers kept as aliases.

**Memory footprint** — if the change adds dependencies or long-lived allocations, measure resident RAM on a UDM/UDR build before merging. Target: <20 MB.

## Phase 4: Verify and Confirm

1. **Acceptance criteria** in `task.md` all met.

2. **Full checks**:
   ```bash
   go fmt ./...
   go vet ./...
   golangci-lint run
   go test -race ./...
   ```

   If `installer` touched:
   ```bash
   bash -n scripts/install-on-unifi-os.sh
   shellcheck scripts/install-on-unifi-os.sh
   ```

   If `server`/`platform`/`installer` touched materially: plan on-device validation on a real UDR/UDM or Pi before merge. Note this in the PR description.

3. **Update `task.md`** with implementation summary and test results.

4. **Ask user for confirmation** before committing.

## Phase 5: Commit and PR

1. **Stage specific paths** — never `git add .`:
   ```bash
   git add cmd/ internal/ docs/ ai_docs/ scripts/ .claude/issues/
   ```

2. **Commit** with conventional format:
   ```bash
   git commit -m "feat: description (fixes #$ARGUMENTS)"
   ```

3. **Push**:
   ```bash
   git push -u origin BRANCH_NAME
   ```

4. **Create PR**:
   ```bash
   gh pr create --repo descoped/dddns \
     --base main \
     --title "feat: description (fixes #$ARGUMENTS)" \
     --body "$(cat <<'EOF'
   ## Summary

   [1-2 sentences]

   fixes #$ARGUMENTS

   ## Changes

   - [Change 1]
   - [Change 2]

   ## Testing

   - [x] go fmt / go vet / golangci-lint / go test -race clean
   - [ ] On-device validation: [UDR/UDM/Pi — note what was verified, or N/A]

   ## Acceptance Criteria

   [Copy from task.md]
   EOF
   )" \
     --assignee @me
   ```

## Phase 6: Post-PR — Tests and Status

1. **Run full suite**:
   ```bash
   go test -race ./... 2>&1 | tail -5
   ```

2. **Update GitHub issue body** — mark all `- [ ]` to `- [x]`, add design doc link:
   ```bash
   gh issue edit $ARGUMENTS --repo descoped/dddns --body "..."
   ```

3. **Post test results as PR comment**:
   ```bash
   gh pr comment PR_NUMBER --repo descoped/dddns --body "$(cat <<'EOF'
   ## Test Results

   - go test -race ./...: PASS (N tests in M packages)
   - golangci-lint run: clean
   - go vet: clean
   - [shellcheck clean if installer touched]
   - [on-device validation: UDR / UDM / Pi — what was verified]

   All acceptance criteria met. Ready to merge.
   EOF
   )"
   ```

4. **Update `task.md`** with final status.

## Phase 7: Review PR

Run `/dd-review PR_NUMBER` for a structured review.

- Critical issues → fix, push, re-review.
- Review passes → proceed to merge.

## Phase 8: Merge

```bash
gh pr view PR_NUMBER --repo descoped/dddns --json title,number,state,url
gh run list --repo descoped/dddns --limit 5
gh pr merge PR_NUMBER --repo descoped/dddns --squash
```

## Phase 9: Post-Merge

1. **Switch to main and pull**:
   ```bash
   git checkout main && git pull origin main
   ```

2. **Archive issue folder**:
   ```bash
   mkdir -p .claude/history
   mv .claude/issues/issue-$ARGUMENTS .claude/history/
   git add .claude/history/issue-$ARGUMENTS .claude/issues/
   git commit -m "docs: archive issue-$ARGUMENTS to history"
   git push origin main
   ```

3. **Update GitHub issue body** — replace `.claude/issues/` paths with `.claude/history/`.

## Rules

- `design.md` and `task.md` are **mandatory** — created before implementation.
- Read every referenced document before designing.
- Track progress in `task.md` — check off as done.
- User confirmation required before commit.
- Issue folder stays in `.claude/issues/` until PR is **merged**; archive to `.claude/history/` only after.
- Use `fixes #$ARGUMENTS` in the PR to auto-close the issue.
- Conventional commit format; no AI attribution.
- Working directory: `.claude/` and `git` commands run from project root. Use `(cd path && ...)` subshell for subdirectory commands.
