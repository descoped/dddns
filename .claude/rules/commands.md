# Commands

Applies to: CLI surface (`cmd/**`) and installer (`scripts/install-on-unifi-os.sh`).

## dddns CLI

```bash
# Core
dddns update [--dry-run] [--force] [--quiet] [--verbose]
dddns config init
dddns config check
dddns config set-mode {cron|serve}   # Switch run mode; rewrites boot script
dddns config rotate-secret [--init]  # Rotate serve-mode shared secret
dddns ip
dddns verify

# Serve mode (same-host bind)
dddns serve                          # Start listener (blocks)
dddns serve status                   # Show last request / auth outcome
dddns serve test                     # Loopback Basic-Auth'd test request

# Security
dddns secure enable                  # Convert to encrypted config
dddns secure test                    # Test encryption
```

## UniFi installer (`scripts/install-on-unifi-os.sh`)

```bash
bash install-on-unifi-os.sh                    # Install or upgrade (preserves current mode)
bash install-on-unifi-os.sh --mode cron        # Fresh install / switch to cron
bash install-on-unifi-os.sh --mode serve       # Fresh install / switch to serve
bash install-on-unifi-os.sh --force            # Reinstall even if version matches
bash install-on-unifi-os.sh --probe            # Privacy-safe self-diagnosis
bash install-on-unifi-os.sh --disable          # Stop update loop; keep binary + config
bash install-on-unifi-os.sh --uninstall        # Remove binary + install dir; preserve config
bash install-on-unifi-os.sh --rollback         # Restore previous version from .prev
```

One-liner for first install:

```bash
bash <(curl -fsL https://raw.githubusercontent.com/descoped/dddns/main/scripts/install-on-unifi-os.sh)
```

The installer prompts for run mode on fresh installs and preserves the detected mode on upgrade. Cron mode pipes output through `logger -t dddns` to systemd-journald; there is no rotating `/var/log/dddns.log`.
