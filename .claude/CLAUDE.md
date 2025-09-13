# CLAUDE.md

This file provides guidance to Claude Code when working with dddns.

## Project Goal

Port the bash script `scripts/update-dns-a-record.sh` to a simple Go CLI that updates Route53 DNS records. Target: UDM7 router running via cron.

## Development Principles

**DO AS LITTLE AS POSSIBLE TO MAKE IT WORK**
- No over-engineering
- No extra features unless explicitly requested
- Match the bash script functionality - nothing more
- Single purpose: update DNS when IP changes
- Optimize for memory-constrained devices (<20MB usage)

## What This Tool Does

1. Check public IP
2. Compare with cached IP
3. Update Route53 if changed
4. Exit

That's it. Run via cron every 30 minutes.

## Commands Needed

```bash
# Build
make build-udm

# Run
dddns update
dddns update --dry-run
dddns update --force

# Config
dddns config init
dddns config check

# Debug
dddns ip
```

## AWS Configuration

- Profile: `route66dns`
- Hosted Zone: `ZBCMVMPX00SYZ`
- Hostname: `route-66.no`
- TTL: 300

## File Locations (UDM7)

- Binary: `/data/dddns/dddns`
- Config: `/data/.dddns/config.yaml`
- IP Cache: `/data/.dddns/last-ip.txt`
- Logs: `/var/log/dddns.log`

## Current Status

✅ Core functionality implemented
✅ Security issues fixed (no hardcoded credentials)
✅ Runs on UDM7
✅ Simple, lean, memory-efficient

## Do NOT Add Unless Asked

- ❌ Metrics/monitoring
- ❌ Web UI
- ❌ Multiple DNS providers
- ❌ Daemon mode
- ❌ Complex retry logic
- ❌ Service discovery
- ❌ Containers
- ❌ Additional commands
- ❌ Extra abstractions