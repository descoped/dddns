# Security Roadmap

**Status:** Partial — the "Already shipped" section tracks what's in `main`; "Open work" items are not yet implemented.
**Confidence:** High — shipped items fact-checked against the code; open items are honest proposals with known trade-offs.
**Last reviewed:** 2026-04-17

## Scope

Credential protection work still open after the UniFi bridge ship. What has already landed is listed up front so it isn't re-planned.

## Out of scope

- The shipped security model for `dddns serve` — see `5_unifi-ddns-bridge.md` §3 (L1–L6 layered defenses). Anything here is supplementary.
- Route53 IAM scoping — already documented in `docs/aws-setup.md`.
- TPM / HSM / hardware-backed key storage. UDM/UDR don't expose a TPM; the complexity isn't justified for dddns's threat model.

## Already shipped (do not re-plan)

| Item | Where |
|---|---|
| AES-256-GCM at rest for all config secrets | `internal/crypto/device_crypto.go` |
| Device-derived encryption key | same |
| `EncryptString` / `DecryptString` primitives for non-credential secrets | same |
| Constant-time credential compare | `internal/server/auth.go` |
| Sliding-window auth lockout (5 fails / 60 s → 5 min block) | same |
| JSONL audit log with size-based rotation | `internal/server/audit.go` |
| Fail-closed `ServerConfig` validation (empty CIDR allowlist refuses startup) | `internal/config/config.go` |
| Encrypted server shared secret (`SecretVault` pattern) | `internal/config/secure_config.go` |
| Server shared-secret rotation | `cmd/config_rotate_secret.go` |
| Scoped Route53 IAM policy (record + action condition keys) | `docs/aws-setup.md` |
| IP validation via stdlib (retired `ip-api.com`) | `internal/commands/myip/myip.go` (`ValidatePublicIP`) |

## Open work

### 1. Random per-install salt

**Current:** Salt is hardcoded in the binary (`internal/crypto/device_crypto.go:142`, value `"dddns-vault-2025"`).

**Change:** Generate 32 random bytes on first `secure enable`, store at `<config_dir>/.salt` with 0400. Derive the encryption key from `deviceID + per_install_salt` instead of `deviceID + hardcoded_salt`.

**Why it matters:** a hardcoded salt means every install's ciphertext lives in the same key-derivation space. A leaked binary + a known device ID → a rainbow table against the vault. Random salt makes each install's key space disjoint.

**Migration:** on first load after upgrade, detect absence of `.salt`, generate one, re-encrypt the vault in place. Transparent to the user.

**Priority:** highest defense gain per line of code.

### 2. Stronger key derivation

**Current:** SHA-256 of `deviceID + salt` — a single hash, not a KDF.

**Change:** PBKDF2-HMAC-SHA256 with ≥100k iterations, or Argon2id with `time=1, memory=64 MiB, parallelism=4`. Argon2 is memory-hard (GPU-resistant); PBKDF2 is CPU-hard only. Pick one; don't mix.

**Why it matters:** SHA-256 is a fast hash. An attacker with the encrypted blob and guessable device-ID inputs (MAC address, hostname) can brute-force the key at billions per second on a GPU. PBKDF2 / Argon2 slows this by 4–6 orders of magnitude.

**UDM caveat:** Argon2id with 64 MiB runs on UDM, but eats notable RAM during decrypt. Decrypt happens once at startup, so the spike is brief. Acceptable.

**Breaking change:** no. Prepend a version byte to the ciphertext so old blobs still decrypt; new blobs write with the new KDF.

### 3. AWS credential rotation

**Current:** `dddns config rotate-secret` rotates the serve-mode shared secret. AWS access/secret keys require re-running `dddns config init`.

**Change:** `dddns config rotate-aws-keys` accepts new keys (flag or prompt), re-encrypts the vault, prints a confirmation. Mirrors `rotate-secret`.

**Why it matters:** AWS best practice is periodic key rotation. A first-class subcommand beats "hand-edit the .secure file" or "run init again, which asks ten other questions."

### 4. Passphrase-protected cron mode (opt-in)

**Current:** No passphrase support. Encryption key is fully device-derived.

**Change:** `dddns secure enable --passphrase` prompts for a passphrase and derives the key from `deviceID + per_install_salt + passphrase`.

**Constraint:** `dddns serve` is a systemd service — it must start without interactive input. Therefore **passphrase mode is incompatible with serve mode**. This is a cron-only feature. `dddns config set-mode serve` on a passphrase-protected config fails with a clear error pointing the user at `rotate-secret` to remove the passphrase.

**Why it matters:** adds a "something you know" factor for users operating dddns on a shared machine. Protects against an attacker who gains both the binary and a copy of `.secure` + the salt.

### 5. Detection beyond the local audit log

**Current:** serve mode writes a local JSONL audit log. `dddns serve status` shows the last request. No external detection.

**Options:**

- **CloudTrail on the hosted zone.** AWS-native; logs every `ChangeResourceRecordSets` call with source IP, signer, timestamp. Independent of dddns, so it survives a full dddns compromise. Single-paragraph docs addition — no dddns code change needed.
- **Shell hook on auth failure.** Already supported via `server.on_auth_failure`. Document common wiring: ntfy, Pushover, a webhook to an incident channel.
- **Optional built-in ntfy / webhook.** Small HTTP POST from the handler on `badauth`, `dnserr`, or `rotate-secret` events. Synchronous with a short timeout so a failed notification doesn't block request processing.

Ship CloudTrail docs now; keep the shell hook as the generic path. Built-in notification only if there's demand — the shell hook covers the use case.

### 6. Memory wiping

**Current:** `crypto.SecureWipe` exists but is unused (`internal/crypto/device_crypto.go:228`, `//nolint:unused`).

**Change:** call `SecureWipe` on decrypted-secret buffers after use. `defer secret.Wipe()` in the handler.

**Why it matters, and why it's low:** Go's GC and string interning mean we can't guarantee every copy of a secret is zeroed. This helps but isn't a complete defense. Low cost, low benefit — do it when we're already touching the crypto package for §1 or §2.

## Priority

1. Random per-install salt (§1).
2. Stronger KDF (§2).
3. AWS key rotation (§3).
4. Passphrase-protected cron mode (§4).
5. Detection hooks (§5) — docs first, built-in only if demanded.
6. Memory wiping (§6) — opportunistic.

## Non-goals

- **TPM / HSM integration.** No TPM on UDM/UDR; substantial platform-specific code and a hard dependency the binary doesn't need. Out of scope.
- **Replace AES-256-GCM.** The cipher is not the bottleneck; KDF and salt are.
- **Config hot-reload.** `rotate-secret` and `set-mode` require a restart of the supervisor-managed `dddns serve`. Hot-reload complicates the failure model for a few seconds of downtime.
