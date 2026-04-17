# Create Issue

Create a GitHub issue for dddns.

## Configuration

```
Repo: descoped/dddns
```

### Workspace Areas

| Area | Path | Description |
|------|------|-------------|
| `core` | `cmd/{update,verify,ip}.go`, `internal/{updater,dns,commands/myip}` | Update flow, Route53 client, public IP detection |
| `cli` | `cmd/` | CLI commands, flags, user-facing UX |
| `server` | `cmd/serve.go`, `cmd/config_{set_mode,rotate_secret}.go`, `internal/{server,bootscript}` | Serve-mode HTTP handler, boot script generator |
| `security` | `internal/config`, `internal/crypto`, `docs/aws-setup.md` | Config encryption, auth, audit log, IAM |
| `platform` | `internal/{profile,wanip}` | UDM/Pi/Linux/macOS/Windows specifics, WAN IP lookup |
| `installer` | `scripts/install-on-unifi-os.sh`, `.goreleaser.yaml`, `Makefile` | Install scripts, release plumbing |
| `docs` | `docs/`, `ai_docs/`, `README.md` | User and design documentation |

## Phase 1: Determine Scope

Ask the user:

1. **Which area(s) are affected?** (can be multiple — see table above)
2. **What type of issue?**
   - Feature (new functionality)
   - Bug (something broken)
   - Enhancement (improve existing)
   - Docs (documentation)
   - Refactor (code restructure)
   - Test (test improvements)

**Before accepting a feature scope**, check the "Do NOT Add Unless Explicitly Asked" list in `.claude/CLAUDE.md`. If the request crosses that list (new daemon mode, metrics, DB storage, etc.), surface the conflict to the user before creating the issue.

**Before accepting a multi-provider scope**, confirm the work is per `ai_docs/0_provider-architecture.md`.

## Phase 2: Gather Requirements

Collect from the user (solution-agnostic — no file paths, no code specifics):

- **Context**: Why is this needed? What's the background?
- **Current State**: What's the problem? Describe observable behavior.
- **Objective**: What should be achieved? Focus on outcomes.
- **Acceptance Criteria**: How do we know it's done? Observable behavior only.

**If design work is needed**:
- Create `.claude/issues/issue-{N}/design.md` after issue creation (Phase 6).
- Design doc must be fully self-contained — copy all relevant specs verbatim, never reference external docs by path. Cross-references inside `ai_docs/` are OK since those are stable; external URLs and paths outside the repo are not.

## Phase 3: Search for Duplicates

```bash
gh issue list --repo descoped/dddns --state open --search "KEYWORDS"
gh issue list --repo descoped/dddns --state closed --search "KEYWORDS"
```

If matches exist, present them to the user and ask how to proceed (comment on existing, new related issue, or skip).

## Phase 4: Select Labels

```bash
gh label list --repo descoped/dddns
```

**Available labels:**

Area (pick 1 or more): `core`, `cli`, `server`, `security`, `platform`, `installer`
Type (pick 1): `bug`, `enhancement`, `docs`, `refactor`, `test`
Priority (optional): `priority: high`, `priority: low`
Status (optional): `blocked`, `needs-design`

## Phase 5: Create Issue

```bash
gh issue create --repo descoped/dddns \
  --title "TITLE" \
  --body "BODY" \
  --label "LABELS"
```

## Phase 6: Organize Issue Documents

After the issue is created (e.g. issue #42):

1. **Create issue folder**:
   ```bash
   mkdir -p .claude/issues/issue-42
   ```

2. **Create design doc** if design work is needed:
   - Write `.claude/issues/issue-42/design.md` with analysis, rationale, and relevant specs copied verbatim from `ai_docs/`.
   - The design doc is the permanent record — treat it as the spec that survives after `ai_docs/` evolves.

3. **Commit**:
   ```bash
   git add .claude/issues/
   git commit -m "docs: organize issue #42 documents"
   ```

## Standard Issue Body

```markdown
## Context

[Why is this needed? Link to `ai_docs/N_*.md` if relevant.]

**Design Doc**: `.claude/issues/issue-{N}/design.md` (if exists)

## Current State

[What's the problem? Describe observable behavior.]

## Objective

[What should be achieved? Outcome, not method.]

## Area

- [ ] `core` / `cli` / `server` / `security` / `platform` / `installer` / `docs`

## Tasks

- [ ] [High-level functional outcome 1]
- [ ] [High-level functional outcome 2]

## Acceptance Criteria

- [ ] [Observable behavior 1]
- [ ] [Observable behavior 2]
```

## Rules

- Issues are solution-agnostic — no file paths, no code specifics.
- Search for duplicates before creating.
- Design docs in `.claude/issues/issue-{N}/` are self-contained; `ai_docs/` cross-references are fine.
- Honour `.claude/CLAUDE.md` "Do NOT Add" list.
- Return the issue URL when done.
