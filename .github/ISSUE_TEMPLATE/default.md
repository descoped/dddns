---
name: General Issue
about: Create an issue for dddns
labels: ''
---

<!--
SOLUTION-AGNOSTIC PRINCIPLES:
- Focus on WHAT and WHY, not HOW
- No file paths (e.g., internal/updater/updater.go)
- No specific code structures (e.g., "add field X to ServerConfig")
- Acceptance criteria describe observable BEHAVIOR, not code changes

Implementation details belong in task files (.claude/issues/issue-X/task.md), not issues.
-->

## Context

<!-- Why is this needed? What's the background? -->

## Current State

<!-- What's happening now? What's the problem? Describe behavior, not implementation. -->

## Objective

<!-- What should be achieved? Focus on outcomes, not methods. -->

## Tasks

<!-- High-level functional outcomes, not implementation steps -->
- [ ] <!-- Example: "dddns rejects CGNAT IPs during IP validation" NOT "add 100.64/10 check to ValidatePublicIP" -->
- [ ]

## Acceptance Criteria

<!-- How do we know this is complete? Observable behavior only. -->
- [ ] <!-- Example: "dddns serve status shows last request" NOT "StatusWriter.Write called" -->
- [ ]

## Area

<!-- Check the areas this issue affects -->
- [ ] `core` — update flow, Route53 client, IP detection
- [ ] `cli` — CLI commands, flags, user-facing UX
- [ ] `server` — serve-mode HTTP handler
- [ ] `security` — config encryption, auth, audit log, IAM
- [ ] `platform` — UDM/Pi/Linux/macOS/Windows specifics
- [ ] `installer` — install scripts, GoReleaser, Makefile
- [ ] `docs` — documentation only

## Related

<!-- Links to related issues, PRs, or docs in ai_docs/ -->
-
