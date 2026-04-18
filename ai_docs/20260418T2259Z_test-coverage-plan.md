# Test coverage plan — v0.3.1 target

**Status:** Planned. Execution ordered by ROI.
**Current coverage:** 54.0% total (measured 2026-04-18 on origin/main at 873a91a + review fixes).
**Target:** ~78% after Tier 1 + Tier 2. Crossing 80% would require testing code that doesn't earn a test — see "Explicit exclusions" below.

## Starting baseline

Run: `go test -coverprofile=/tmp/cov.out ./... && go tool cover -func=/tmp/cov.out`

| Package | Coverage | Assessment |
|---------|----------|------------|
| `internal/bootscript` | 100% | Complete |
| `internal/commands/myip` | 89% | Complete |
| `internal/server` | 87% | Complete |
| `internal/dns` | 85% | Good; `parseAWSError` 30% is the one gap |
| `internal/updater` | 74% | `updateWithResolver` branches incomplete |
| `internal/wanip` | 70% | OS-branch limited (normal) |
| `internal/profile` | 67% | OS-branch limited (normal) |
| `deploy/aws-lambda` | 55% | `main.go` inherently untestable; handler + ssm well-covered |
| `internal/config` | 54% | Missing path.go + secure_config.go tests |
| `internal/crypto` | 44% | OS-specific collectors dominate; core primitives 70–100% |
| `cmd/` | 29% | **The real gap — most cobra RunEs have no tests** |
| `internal/verify` | 0% | Real business logic, zero tests |
| `internal/version` | 0% | Trivial getters — skip |
| `main.go` | 0% | Entry point — skip |

## Philosophy — what earns a test

A test must catch at least one of:

1. **Invariants** — "this can never happen" assertions that catch silent regressions after refactoring.
2. **Security boundaries** — auth checks, constant-time compare, L6 source-IP enforcement, IAM conditions, never-trust-`myip`.
3. **Error paths with distinct behavior** — a fail-closed path that must stay fail-closed under error conditions different from the happy path.
4. **Non-obvious branches** — auto-detect resolving differently per profile, fallback chains, parse-and-validate logic.
5. **Regressions we've already hit or credibly could** — the SO_BINDTODEVICE discovery, the curl `404000` bug, the UDR7 policy-table fallback, `config.yaml` permissions enforcement.

If a test doesn't hit any of those, it's probably test-ware: maintenance cost without defect-catching value.

## What does NOT earn a test

1. **`rootCmd.Execute()` wrapper** — tests cobra itself, not our code.
2. **`version.GetVersion()` / `GetFullVersion()`** — string concatenation on build-time constants.
3. **OS-specific device-ID collectors** (`deviceIDLinux`, `deviceIDDarwin`, `deviceIDWindows`) off their host — testing through mocks tests the mock, not the production code.
4. **Cobra flag-registration `init()` functions** — framework boilerplate; a test would just re-read the flag table.
5. **Trivial constructors** like `defaultResolver` that assign function values with no branches.
6. **`fatalf`** — wraps `os.Exit(1)` by design; testing requires subprocess forking and proves nothing beyond "os.Exit works."
7. **`fmt.Fprintln` wrappers** like `printSetModeInstructions`, `printNewSecret`, `maskKey` — a test would copy the format string verbatim; any format change breaks the test without breaking the product.
8. **Happy-path-only "does it compile?" tests** — the typechecker handles that.

## Tier 1 — real missing coverage (7 test files, ~250 LoC)

Each row is a standalone commit. Each tested against `go test -race` before moving to the next.

| # | Test file | What it covers | Why it matters |
|---|-----------|---------------|----------------|
| 1 | `internal/verify/verify_test.go` | `Run` happy path; each distinct failure (IP mismatch, DNS mismatch, zero resolvers returning, per-resolver timeout). httptest for checkip stand-in, stub DNS resolver for dig. | 0% → ~85%. Real business logic. `dddns verify` output is what users read to decide whether their setup is working. Silent mis-reporting here = user thinks they're good when they aren't. |
| 2 | `cmd/config_check_test.go` | `runConfigCheck` against: valid config, wrong file permissions (rejected), malformed YAML (rejected with clear error), missing required field. Captures stdout/stderr. | `dddns config check` is the first command every user runs. Silent misbehaviour here hides everything downstream. |
| 3 | `cmd/config_init_test.go` | `runConfigInit` non-interactive: creates file with `0600` permissions; refuses to overwrite existing without `--force`; `--force` overwrites in place. | Config creation is one-shot — breaking the first-time-setup experience is a support ticket magnet. |
| 4 | `cmd/secure_test.go` | `runEnableSecure` round-trip: plaintext YAML → encrypted `.secure` → read back → field values match. `runTestSecure` against a freshly-encrypted file reports success. | Encrypted-at-rest is a claim the README and docs make. Not having a test that proves the round-trip is a credibility gap. |
| 5 | `cmd/serve_status_test.go` | `runServeStatus` with: valid status.json present, file absent (clear "never been called" message), malformed JSON (non-fatal). | Status reporter is a user-facing diagnostic; a silent wrong read misleads triage. |
| 6 | `internal/config/path_test.go` | `SetActivePath` / `ActivePath` round-trip; default-path resolution when unset. | Thin file but it IS the API contract between `cmd/` and `internal/config`. Currently covered only implicitly. |
| 7 | `internal/config/secure_config_test.go` | Encrypted write → read round-trip, 0400 permission enforcement at Load time, AES-GCM tamper detection (flip one byte → decrypt fails). | `internal/crypto` primitives are tested; the config-layer wrapping around them isn't. Closes the "encryption works in crypto but maybe not in config" gap. |

## Tier 2 — plug specific gaps in already-covered files (no new files, ~50 LoC)

Extend existing test files rather than creating new ones.

| Package | Target function | Current → After | Additions |
|---------|----------------|-----------------|-----------|
| `internal/dns` | `parseAWSError` | 30% → 80% | Both XML error-body shapes (short form + ErrorResponse form) + malformed body fallback returning the HTTP status + raw body |
| `internal/crypto` | `deviceIDFallback` | 0% → 100% | Call with `HOSTNAME` set, assert non-empty + deterministic across two calls |
| `internal/updater` | `updateWithResolver` | 65% → 85% | DNS-matches-current branch (nochg-dns path), DryRun with non-empty DNS comparison, cache-write-failure path (read-only tmpdir) |
| `internal/server` | `status.Write` | 57% → 85% | Force the tmp-file rename failure (read-only parent dir), verify the error bubbles and no partial status file lands |

## Tier 3 — explicit exclusions (would be test-ware)

| File / function | Why skipped |
|-----------------|-------------|
| `main.go::main` | 3 lines: `rootCmd.Execute() + os.Exit(1)`. Tests cobra. |
| `internal/version/version.go` | `return "dddns " + Version`. Tests string concatenation on build-time constants. |
| `cmd/root.go::Execute` | `return rootCmd.Execute()`. One line. |
| `cmd/root.go::fatalf` | Calls `os.Exit(1)`. Would need subprocess test machinery that proves nothing beyond "os.Exit works." |
| `deviceIDLinux` / `deviceIDDarwin` / `deviceIDWindows` | Only one runs per host. Testing through mocks means the mock becomes the thing under test, not the real code. Covered de facto by CI running on actual macOS/Linux. |
| `defaultResolver` | Assigns function values with no branches. |
| Cobra `init()` flag registration | Framework boilerplate; a test would re-enumerate the flag table. |
| `printSetModeInstructions`, `printNewSecret`, `maskKey` | Format strings. Any test copies the format; format changes break the test without breaking the product. |

## Expected outcome

| Package | Before | After Tier 1 | After Tier 2 |
|---------|--------|--------------|--------------|
| `internal/verify` | 0% | **80%** | 80% |
| `cmd/` | 29% | **60%** | **65%** |
| `internal/config` | 54% | **78%** | **80%** |
| `internal/crypto` | 44% | 44% | **55%** (OS-branch ceiling) |
| `internal/dns` | 85% | 85% | **90%** |
| `internal/updater` | 74% | 74% | **85%** |
| `internal/server` | 87% | 87% | **89%** |
| **Total project** | **54%** | **~72%** | **~78%** |

To cross 80% we'd have to test the Tier 3 items. Recommend against — those tests create maintenance cost without catching real bugs. A codebase with 78% of-the-right-kind coverage is meaningfully stronger than one with 85% that includes 7% test-ware.

## Execution order

1. `internal/verify/verify_test.go` — biggest ROI; lifts both the internal package and `cmd/verify` together.
2. `cmd/config_check_test.go` — most user-facing validation surface.
3. `cmd/secure_test.go` + `internal/config/secure_config_test.go` — closes the "encryption claim" gap. Bundle into one commit.
4. `cmd/config_init_test.go` + `cmd/serve_status_test.go` — remaining high-signal cmd/ additions.
5. `internal/config/path_test.go` — small file; quick win.
6. Tier 2 plugs — expand four existing test files with targeted cases.

After each commit: `go test -race ./... && golangci-lint run ./...` must stay green. Coverage number re-captured after Tier 1 and after Tier 2 — if we fall short of projections, diagnose before moving on (don't add fluff tests to hit a number).

## Rules for new tests

- **Name tests after the invariant or behavior**, not the method. `TestConfigCheck_RejectsWorldReadablePlaintext` beats `TestRunConfigCheck_Case5`.
- **One assertion per behavior** — subtests for edge cases, not one giant `TestEverything`.
- **Table-driven only when the cases are genuinely homogenous.** If each case needs different setup, separate functions are clearer.
- **No mocks of `internal/dns` internals** — use the exported `DNSClient` interface (already 2-method by design per `ai_docs/0_provider-architecture.md`).
- **Temp dirs via `t.TempDir()`** — never write outside it.
- **No external network calls.** `httptest.Server` for HTTP, stub functions for OS probes, no real Route53 / SSM.
- **Privacy-safe fixtures only** — RFC 5737 (TEST-NET-1/2/3), RFC 2606 (`example.com`), fabricated zone IDs (`Z1ABCDEFGHIJKL`, `Z123`).

## Acceptance

This plan is done when:

1. All Tier 1 commits land on `main` with `go test -race` passing.
2. Tier 2 plugs land (or explicit decision to skip them after reviewing the post-Tier-1 number).
3. Total coverage is ≥75%, concentrated in code that changes frequently.
4. No tests were added for anything in the Tier 3 exclusion list.
5. `v0.3.1` is tagged including the coverage work.
