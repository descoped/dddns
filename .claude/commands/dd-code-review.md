# Clean Code Review $ARGUMENTS

Perform a deep code quality analysis using clean code engineering principles. Identifies violations of SRP, DRY, dead code, stubs, complexity, coupling, and naming issues. Produces a structured report — does NOT create GitHub issues (use `/dd-issue` for that).

## Scope

`$ARGUMENTS` determines what to review:

| Argument | Scope |
|----------|-------|
| `core` | `internal/updater/`, `internal/dns/`, `internal/commands/myip/` |
| `cli` | `cmd/` (update.go, verify.go, ip.go, config.go, root.go, secure.go) |
| `server` | `internal/server/`, `internal/bootscript/`, `cmd/serve.go`, `cmd/config_set_mode.go`, `cmd/config_rotate_secret.go` |
| `security` | `internal/config/`, `internal/crypto/` |
| `platform` | `internal/profile/`, `internal/wanip/` |
| `installer` | `scripts/install-on-unifi-os.sh` |
| `all` | Full codebase — work through one area at a time (order below) |
| `path/to/file` | Single file or directory |

If no argument, ask the user.

## Method

Read files using the Read tool — don't just grep. Understand context, call chains, and module boundaries before flagging anything. **Verify before reporting** — false positives waste time and erode trust.

Work in dependency order: `internal/{crypto,profile,wanip}` → `internal/{config,bootscript}` → `internal/{dns,commands/myip}` → `internal/{updater,server}` → `cmd/` → `scripts/`.

For each file or module, evaluate against the categories below. Skip files that are trivially clean (small, focused, well-named).

---

## Categories

### 1. Single Responsibility Principle (SRP)

A function, type, or module should have one reason to change.

**What to look for:**

- Functions doing multiple unrelated things (fetch + transform + persist in one function)
- Types with mixed concerns (a struct that's both a domain model and a DTO)
- Modules mixing abstraction levels (parsing next to business rules)
- God objects — types or components doing everything
- **[go]** Route handlers or `cobra` command `RunE`s containing business logic instead of delegating to `internal/` packages

**How to report:**
> `file:line` — `FunctionOrType`: does X, Y, and Z. Split into [suggestion].

### 2. Don't Repeat Yourself (DRY)

Every piece of knowledge should have a single, unambiguous representation.

**What to look for:**

- Copy-pasted code blocks (same logic in multiple handlers, tests, or adapters)
- Repeated patterns that differ only in type name or field
- Duplicated constants or magic numbers across files
- Similar types that could share a common base or generic
- **[go]** Repeated error handling boilerplate (`if err != nil { return err }` with identical wrapping)
- **[go]** Duplicated `context.WithTimeout` + `defer cancel()` patterns that could factor into a helper

**How to report:**
> Pattern `X` is duplicated in `file1:line`, `file2:line`, `file3:line`. Extract to [shared location].

### 3. Dead Code & Orphans

Code that is never called, never reachable, or no longer needed.

**What to look for:**

- Unused functions, methods, types, enums, or enum variants
- Unused imports (should already be caught by `go vet`; flag if the tool missed them)
- Unreachable `switch` cases or `if`-branches
- Commented-out code blocks left behind
- Files that nothing imports or references
- Public API surface that has no external consumers
- **[go]** Unexported functions with no callers in the package
- **[go]** `//nolint:unused` hiding real dead code

**How to report:**
> `file:line` — `symbol_name` is never referenced. Safe to remove. (Verified: searched all call sites.)

### 4. Stubs & Incomplete Code

Placeholder implementations that were never finished.

**What to look for:**

- `TODO`, `FIXME`, `HACK`, `XXX`, `TEMP`, `STUB` comments
- Empty function/method bodies or bodies that just return a default/placeholder
- Empty `if err != nil { return err }` variants that swallow context
- Functions that always return hardcoded values or empty collections
- **[go]** `panic("not implemented")` or `// TODO` with empty function body
- **[go]** `errors.New("TODO")` sentinel errors

**How to report:**
> `file:line` — `function_name`: stub/TODO found. Status: [likely forgotten | intentional placeholder | blocking on X].

### 5. Complexity

Code that is harder to understand, test, or modify than necessary.

**Thresholds for Go:**

| Metric | Threshold |
|--------|-----------|
| Long function | >40 lines |
| Deep nesting | >3 levels |
| Too many params | >4 |
| Long file | >400 lines |

**Universal checks:**

- Complex conditionals: boolean expressions with >3 terms — extract to named function
- Long `switch` chains: >6 cases — consider a lookup table or dispatch
- Primitive obsession: using `string` or `int` where a typed wrapper would add safety (e.g., a hostname is not just a string)

**How to report:**
> `file:line` — `function_name`: [metric] (e.g., 67 lines, 5 nesting levels, 7 parameters). Simplify by [suggestion].

### 6. Coupling & Cohesion

Modules should be loosely coupled (few external dependencies) and highly cohesive (everything inside belongs together).

**What to look for:**

- **Tight coupling**: Module A directly constructs or calls internals of Module B instead of using an interface
- **Circular dependencies**: A depends on B depends on A
- **Feature envy**: A function that uses more data from another module than its own
- **Shotgun surgery**: Changing one concept requires touching >3 files
- **Low cohesion**: A package containing unrelated functions/types that don't interact
- **Leaky abstractions**: Implementation details exposed in public interfaces
- **[go]** Package calling another package's internals instead of using an exported interface
- **[flat project]** `cmd/` importing from `cmd/` (CLI commands should be siblings, not dependencies)
- **[dddns-specific]** `internal/` packages importing from `cmd/` (wrong direction)
- **[dddns-specific]** Anything outside `internal/server/` directly talking to the audit log or status file

**How to report:**
> `module_a` is tightly coupled to `module_b` via [specific dependency]. Decouple by [suggestion].

### 7. Naming & Clarity

Code should read like well-written prose. Names should reveal intent.

**What to look for:**

- Single-letter variables outside of closures/iterators (`x`, `s`, `v` as function params)
- Misleading names (function named `GetX` that also modifies state)
- Inconsistent naming (same concept called `item` in one place and `entry` in another)
- Generic names that don't convey purpose (`data`, `result`, `value`, `info`, `temp`, `helper`, `utils`)
- Type names that don't match their role (a service called `Manager`, a DTO called `Model`)
- **[go]** Boolean variables/functions without `Is`/`Has`/`Can`/`Should` prefix
- **[go]** Abbreviated names where the full word is clearer (`cfg` vs `config` is idiomatic in dddns — don't flag this specifically; `mgr`, `hdlr`, `svr` are not)

**How to report:**
> `file:line` — `old_name`: rename to `suggested_name` for clarity. Reason: [why current name is misleading/unclear].

---

## Stack-Specific Checks

### Go

- Ignored errors (`val, _ := riskyCall()` without justification)
- Missing error context (`return err` instead of `fmt.Errorf("context: %w", err)`)
- `panic()` in library code — return errors instead (exception: recovered panics inside `http.Handler` that map to dyndns `911` are intentional)
- Missing `context.Context` propagation in functions that do I/O (AWS, HTTP, file)
- Goroutine leaks (fire-and-forget goroutines without lifecycle management; tests must join before returning)
- `interface{}` / `any` where a concrete type or generic would be safer
- `init()` functions with side effects

**Check commands:**
```bash
go fmt ./...
go vet ./...
golangci-lint run
go test -race ./...
```

---

## Report Format

Present findings grouped by category, sorted by severity within each group.

```markdown
## Clean Code Review: [scope]

**Files reviewed**: N
**Findings**: N total (N critical, N refactor, N minor)

### SRP Violations (N)

| # | File | Symbol | Issue | Suggestion |
|---|------|--------|-------|------------|
| 1 | `path/to/file:45` | `FunctionOrType` | Does X, Y, and Z | Extract X into [suggestion] |

### DRY Violations (N)

| # | Files | Pattern | Occurrences | Suggestion |
|---|-------|---------|-------------|------------|
| 1 | `path/*.go` | Repeated pattern | N files | Extract to [shared location] |

### Dead Code (N)

| # | File | Symbol | Evidence |
|---|------|--------|----------|
| 1 | `path/to/file:120` | `symbolName` | No callers found in codebase |

### Stubs & TODOs (N)

| # | File | Type | Content |
|---|------|------|---------|
| 1 | `path/to/file:88` | TODO | "description" |

### Complexity (N)

| # | File | Symbol | Metric | Suggestion |
|---|------|--------|--------|------------|
| 1 | `path/to/file:200` | `functionName` | 72 lines, 5 nesting levels | Extract sub-operations |

### Coupling Issues (N)

| # | Modules | Issue | Suggestion |
|---|---------|-------|------------|
| 1 | `pkg_a` -> `pkg_b` | Direct internal access | Use exported interface |

### Naming Issues (N)

| # | File | Current | Suggested | Reason |
|---|------|---------|-----------|--------|
| 1 | `path/to/file:15` | `data` | `descriptiveName` | Generic name hides intent |
```

After presenting the report, ask the user:
- Which findings to fix now (you'll fix them directly)
- Which to create as issues (`/dd-issue`)
- Which to skip

## Rules

- **Verify before reporting** — read the full call chain. If a function looks unused, search for all references before flagging it.
- **Read actual files** — understand context, not just pattern matches.
- **No false positives** — if you're unsure, investigate deeper or skip it. Every finding must be defensible.
- **Actionable findings only** — each finding must have a concrete suggestion. "This could be better" is not actionable.
- **Respect existing patterns** — if the codebase consistently uses a pattern (even if not your preference), don't flag it unless it causes real problems.
- **Skip trivial files** — config files, generated code, test fixtures, and small utility files (<30 lines) rarely need deep review.
- **Working directory** — use subshell `(cd subdirectory && ...)` for commands in subdirectories.

### Project-Specific Rules

- **Honour `.claude/CLAUDE.md` "Do NOT Add" list.** Don't suggest adding retry logic, metrics, a plugin system, hot-reload, DB storage, or service discovery — these are explicitly out of scope. Flagging their absence is a false positive.
- **Minimal footprint is a design goal.** UDM target is <20 MB resident. Don't suggest lazy-init, connection pools, or heavier dependencies without a footprint justification.
- **`updater.DNSClient` is intentionally 2-method** (per `ai_docs/0_provider-architecture.md`). Don't flag it as "interface too narrow" — widening requires a design change, not a code-review fix.
- **Serve-mode security posture is intentional.** `internal/server/auth.go`'s constant-time compare + sliding-window lockout, the CIDR allowlist, and the "never trust `myip` query param" rule are L1–L6 threat-model defenses (see `ai_docs/5_unifi-ddns-bridge.md` §3). Don't suggest simplifications that weaken any layer.
- **systemd supervises `dddns serve` on UDM.** Don't suggest adding in-process supervision, restart loops, or a shell-supervisor pattern — that's what systemd is for.
- **No backward-compatibility shims.** If code looks like a legacy alias or deprecated wrapper, flag it for deletion, not preservation.
- **Single-Route53 is the current reality.** Don't suggest "abstract to support other providers" as a finding — that's planned via `ai_docs/0_provider-architecture.md` and is design work, not code-review work.

### Quality Check Commands (per area)

- **`core` / `cli` / `server` / `security` / `platform`**: `go fmt ./... && go vet ./... && golangci-lint run && go test -race ./...`
- **`installer`**: `bash -n scripts/install-on-unifi-os.sh && shellcheck scripts/install-on-unifi-os.sh`
