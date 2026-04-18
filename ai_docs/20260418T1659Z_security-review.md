# v0.2.0 security review — is dddns safe on UDR7?

**Reviewed:** 2026-04-18
**Scope:** Full codebase (`internal/` + `cmd/` + `scripts/`)
**Focus:** Security first; clean code second
**Status:** Report + apply — findings triaged into "fix now" / "deferred" / "not an issue" buckets.
**Verdict (short answer):** YES — serve mode and the update flow are safe to announce to the UniFi community for UDR7 / UDM-family deployment, with a handful of one-line hardening fixes applied in this review.

Review performed by four parallel deep-read agents, one per security-critical slice:

| Agent | Scope |
|---|---|
| A | `internal/crypto/` + `internal/config/` |
| B | `internal/server/` (all files + tests) |
| C | `internal/dns/` + `internal/updater/` + `internal/commands/myip/` |
| D | `cmd/` + `internal/wanip/` + `internal/profile/` + `internal/bootscript/` |

Each agent returned a structured findings table. The merged results are in the three buckets below.

---

## Bucket 1: Fix now (applied in this session)

These are the clear-win hardening deltas. All are small diffs, all close real gaps, all apply to the UDR7 deployment.

### F1. `myip.GetPublicIP` — HTTP status + body size limit *(Agent C, IMPORTANT #1 + #2)*

**File:** `internal/commands/myip/myip.go`

Today `GetPublicIP` calls `io.ReadAll(resp.Body)` without checking `resp.StatusCode` or bounding the read size. A 500 error page with a garbled IP buried in it could be parsed as a valid response, and a hostile MITM could return an unbounded payload (UDM has ~20 MB RAM budget).

Fix: check status, wrap the reader in `io.LimitReader` (64 bytes — checkip returns ~15).

### F2. `route53.do()` — response body size limit *(Agent C, IMPORTANT #2)*

**File:** `internal/dns/route53.go`

Same class as F1. Route53 responses are typically <10 KB; a 1 MB cap is generous and prevents OOM if the endpoint is compromised or a proxy injects a large payload.

### F3. Plaintext `config.yaml` permission enforcement at `Load()` *(Agent A, BUG/CORRECTNESS)*

**File:** `internal/config/config.go`

`LoadSecure()` rejects a `config.secure` whose mode is not 0600 or 0400. `Load()` for plaintext `config.yaml` does NOT enforce this — a file at 0644 with AWS credentials silently loads. This closes a local-privilege-escalation window (other local accounts reading AWS secret keys).

Fix: require mode bits to be exactly 0600.

### F4. `strings.EqualFold` for hostname comparison *(Agent B, IMPORTANT #3)*

**File:** `internal/server/handler.go`

DNS hostnames are case-insensitive per RFC 1035 §2.3.3. The handler uses `!=` against `h.cfg.Hostname`, so `HOME.example.com` from a theoretical mis-configured inadyn would 404 with "nohost". Easy fix, adds robustness without weakening security (hostname is already compared against a config value, not an attacker-controlled trust anchor).

### F5. Log audit/status write failures to stderr *(Agent B, BUG #7)*

**File:** `internal/server/handler.go`

`h.emit(entry)` silently drops errors from `audit.Write` and `status.Write`. The ignore was deliberate ("must not prevent responding to the client"), but operational visibility matters — silent audit loss is the kind of thing that erodes investigation after the fact. Log the failure to stderr (which systemd journals) without changing the response path.

### F6. `exec.CommandContext` timeouts for device-ID retrieval *(Agent A, IMPORTANT)*

**File:** `internal/crypto/device_crypto.go`

`deviceIDDarwin()` / `deviceIDWindows()` run `system_profiler` / `ioreg` / `wmic` with no timeout. A hung system_profiler (seen in the wild on macOS when IORegistry is locked) blocks startup indefinitely. Not a UDR7 issue (Linux path never hits these), but it affects developer machines and would surface as "dddns hangs on install" for non-UDM users.

Fix: 5-second context timeout on each `exec`.

---

## Bucket 2: Deferred (acknowledged, not fixed in this review)

These either don't apply to UDR7 or require a larger refactor than the v0.2.0 window justifies. Each is flagged for v0.2.1+ follow-up.

| Finding | Source | Why deferred |
|---|---|---|
| Generic-Linux device-ID fallback weakness (hostname-only) | Agent A, C2 | UDR7 / UDM always have `/proc/ubnthal/system.info` with `serialno=`, so UDR7 deployments never hit the weak fallback. Worth tightening for Pi / generic Linux users — file a separate issue. |
| `SaveSecure()` chmod-then-write TOCTOU | Agent A, BUG | Low risk — `.secure` parent dir is `/data/.dddns/` (mode 700), so a concurrent reader requires root already. A full refactor to `O_EXCL` + `os.Rename` is a bigger change than v0.2.0 allows. |
| Audit log rotation race | Agent B, Important #2 | Microsecond TOCTOU window under heavy load; threat model doesn't include local readers of the audit trail. |
| Panic recovery path untested | Agent B, BUG #4 | Recovery code is defensive (handler.go:54-60); worth a test but not install-blocking. |
| Status file permissions documentation | Agent B, Important #1 | Go's `os.CreateTemp` guarantees 0600; `os.Rename` preserves. Safe today; add doc comment in v0.2.1 so future refactors don't regress. |
| wanip fallback interface ordering | Agent D, Important | Multi-tenant / Docker-on-router scenarios; UDR7 has deterministic kernel enumeration. The `Server.WANInterface` config override is already a first-class escape hatch — document it in `docs/`. |
| MAC-address-derived device key | Agent D, Important | By design. The `.secure` file is mode 0400 root-owned; MAC address readability doesn't give an attacker file access. Design rationale belongs in `docs/`, not a code fix. |
| MyIPClaimed field naming | Agent B, Clean Code #6 | Renaming a public struct field ripples through audit-log schema; defer to a future minor version if we rev the audit format for other reasons. |

---

## Bucket 3: Not an issue (validated safe)

The agents explicitly noted these as suspicious-looking-but-fine. Listing for efficiency so future reviewers don't re-chase them.

- **Authorization-header credential handling** (`internal/dns/sigv4.go`) — credentials used only inside the signing function, never logged. Go cannot truly zero string memory; this is an accepted language-level limitation and there's no practical mitigation short of switching to byte slices everywhere (not worth it).
- **XML unmarshaling of Route53 responses** — Go's `encoding/xml` does not process external entities by default. No custom `UnmarshalXML` methods. Billion-laughs safe.
- **No `InsecureSkipVerify` anywhere** in the codebase.
- **Boot script template injection** — all user-configurable fields in `internal/bootscript/` use `%q` quoting for shell-safe interpolation; the cron schedule + log path are hardcoded in `DefaultUnifiParams()`.
- **ConstantTimeCompare used correctly** for the Basic Auth secret. Username is intentionally not constant-time (it's the same on every request, and dyndns v2 doesn't allow per-user secrets).
- **CIDR parsing fails closed** — empty allowlist → reject all; malformed CIDR in config → startup fails validation.
- **Handler ignores `myip` query parameter** (L6 threat-model defense confirmed in handler.go:125) — reads local WAN IP via the wanip hook and passes it via `updater.Options.OverrideIP`.
- **Cache file writes are atomic** via `os.WriteFile` (POSIX rename semantics).
- **HTTP timeouts on serve-mode listener** are sane — ReadHeaderTimeout=5s, ReadTimeout=10s, WriteTimeout=35s, IdleTimeout=30s.
- **Graceful shutdown** via context cancellation + `http.Server.Shutdown` with 5s window.
- **Systemd unit generated by bootscript** enables `ProtectSystem=strict` / `ProtectHome=true` / `PrivateTmp=true` (see bootscript serveTail).
- **Force flag** skips cache check only, NOT credential or config validation.
- **Dry-run flag** is honored end-to-end: no cache write, no Route53 UPSERT, no side effects.
- **SigV4 signer** validated against AWS's documented reference vector (`sigv4_test.go` TestDeriveSigningKey_AWSReference).
- **Cache file** supports both YAML and bare-IP formats for backward compat with v0.1.1 installs — tested.

---

## Final verdict — is this safe for UDR7?

**Yes.** The layered threat model in `ai_docs/5_unifi-ddns-bridge.md` §3 is correctly implemented:

| Layer | Defense | Status |
|---|---|---|
| L1 (friendly local process) | — (expected use) | N/A |
| L2 (other processes on router) | CIDR allowlist, loopback-only bind | ✓ enforced + tested |
| L3 (LAN-side attacker) | Loopback-only bind, CIDR allowlist | ✓ enforced + tested |
| L4 (WAN-side attacker) | Loopback-only bind (unreachable from WAN) | ✓ enforced + tested |
| L5 (credential brute force) | Constant-time compare + sliding-window lockout (5 fails in 60s → 5-min lockout) | ✓ enforced + tested |
| L6 (myip spoofing) | Handler ignores `myip` param, reads local WAN IP | ✓ enforced + tested |

Core update flow (local-IP → SigV4 → Route53 UPSERT) is end-to-end integrity-preserving. The SigV4 implementation passes AWS's reference vector. No credentials leak through logs or error paths. Config files are permission-enforced (0400 for `.secure`, 0600 for plaintext after F3 fix). Device-derived encryption is appropriate for a single-admin router (the MAC-based key is readable by any local process with file access, but `/data/.dddns/config.secure` is 0400 root, so the scenario doesn't compose).

**Blockers before announcing v0.2.0:** none once the six Bucket-1 fixes land.

**What non-expert users should be told in the community post:** the four items already in `ai_docs/20260418T1650Z_community-release-plan.md` "What to explicitly NOT promise" section.
