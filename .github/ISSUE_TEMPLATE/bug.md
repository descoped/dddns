---
name: Bug Report
about: Report a bug in dddns
labels: bug
---

## Description

<!-- Clear, concise description of the bug -->

## Steps to Reproduce

1.
2.
3.

## Expected Behavior

<!-- What should happen? -->

## Actual Behavior

<!-- What actually happens? Include log output if relevant. -->

## Environment

| Field | Value |
|-------|-------|
| Platform | <!-- UDM/UDR • Raspberry Pi • Linux • macOS • Windows • Docker --> |
| Run mode | <!-- cron • serve --> |
| dddns version | <!-- output of `dddns --version` --> |
| OS version | |

## Logs

<!--
- cron mode: /var/log/dddns.log (or journald)
- serve mode: `journalctl -u dddns` (daemon) and `/var/log/dddns-audit.log` (requests)
-->

```
<paste relevant lines>
```

## Area

- [ ] `core` — update flow, Route53 client, IP detection
- [ ] `cli` — CLI commands, flags, user-facing UX
- [ ] `server` — serve-mode HTTP handler
- [ ] `security` — config encryption, auth, audit log, IAM
- [ ] `platform` — UDM/Pi/Linux/macOS/Windows specifics
- [ ] `installer` — install scripts, GoReleaser, justfile

## Additional Context

<!-- Any other relevant information -->
