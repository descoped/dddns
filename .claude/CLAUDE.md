# CLAUDE.md

Guidance for Claude Code when working with dddns.

## CRITICAL RULES

**NEVER COMMIT WITHOUT EXPLICIT PERMISSION**

- When the user says "do not commit" or indicates this is a planning/design session, you MUST NOT make any commits.
- Always wait for explicit approval before making design decisions or architectural changes.
- The developer is the decision-maker; Claude Code is an assistant.
- Present options and recommendations; let the user choose.
- If in doubt, ASK before making changes.

## Project Overview

**dddns** (Descoped Dynamic DNS) is a lightweight CLI tool for updating AWS Route53 DNS A records with dynamic IP addresses. Originally created to replace a bash script on UDM7 routers, it has evolved into a cross-platform solution while maintaining its core simplicity.

Repository: <https://github.com/descoped/dddns> · Issues: <https://github.com/descoped/dddns/issues>

## Development Principles

**DO AS LITTLE AS POSSIBLE TO MAKE IT WORK**

- No over-engineering.
- No extra features unless explicitly requested.
- Single purpose: update DNS when IP changes.
- Optimize for memory-constrained devices (<20 MB resident on UDM).
- Maintain backward compatibility with existing deployments.
- Fail fast with clear error messages; no retries.

## Do NOT Add Unless Explicitly Asked

- ❌ Metrics / monitoring / telemetry.
- ❌ Web UI or user-facing REST API (the serve-mode `/nic/update` endpoint is a push receiver for inadyn, not a user-facing API).
- ⚠️ Multiple DNS providers — only per the design in `ai_docs/0_provider-architecture.md` (HTTP-only). Do not add a provider outside that framework.
- ❌ Complex retry logic.
- ❌ Service discovery.
- ❌ Container orchestration.
- ❌ Database storage.
- ❌ Configuration hot-reload (rotate-secret / set-mode restart the supervisor instead).
- ❌ Plugin system.
- ❌ TPM / HSM integration.

## Rules index

Topic-scoped rules live under `.claude/rules/` and are auto-discovered by Claude Code. Edit the topic files directly; this index is a signpost for humans browsing the repo.

- `architecture.md` — core flow, component map, cross-cutting security invariants
- `commands.md` — CLI surface and UniFi installer flags
- `platform.md` — per-OS paths and config formats
- `lambda.md` — AWS Lambda deployment, IAM scoping, secret rotation
- `serve-mode.md` — on-device listener, systemd supervision, secret rotation
- `build-release.md` — build, test, release process

## Entry-point files

- `main.go` — entry point, minimal
- `cmd/root.go` — command setup, config loading
- `cmd/update.go` — core update logic
- `internal/profile/profile.go` — platform detection
- `internal/crypto/device_crypto.go` — encryption implementation
- `.goreleaser.yaml` — release configuration
- `justfile` — build automation (`just --list`)
- `ai_docs/0_provider-architecture.md` — the design framework for any future non-Route53 provider
