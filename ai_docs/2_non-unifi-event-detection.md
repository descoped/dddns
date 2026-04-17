# Non-UniFi Event-Driven IP Change Detection

## Scope

Make WAN IP change detection near-instant on platforms other than UniFi OS.

## Out of scope

- UniFi Dream devices (UDR, UDM) — handled by `dddns serve` + inadyn push. See `5_unifi-ddns-bridge.md`.
- Changing the default cron behavior on any platform — polling remains the fallback everywhere.
- Adding a background daemon to the dddns binary. Platform-specific hooks invoke `dddns update`; no always-on process except `dddns serve` (which is UniFi-only).

## Verification needed before promoting any recipe

The hook scripts below are documented from each platform's hook contract, not copy-paste snippets validated on a running system. Before promoting any of them to an official install path or linking them from user-facing docs:

- **Confirm hook directory location** on the target distro. `dhcpcd` hook paths differ between Debian (`/lib/dhcpcd/dhcpcd-hooks/`) and RPM-family distros; Raspberry Pi OS and Debian agree.
- **Confirm WAN interface name.** `eth0` / `wlan0` / `enp0s25` / `ppp0` vary by hardware and by whether the distro uses classic naming or `systemd-networkd` predictable names. The scripts assume classic `eth0`/`wlan0` — update per system.
- **Confirm the environment variables** available in the hook context. `$reason`, `$interface` (dhcpcd), `$PPP_IFACE` (ppp), and NetworkManager's `$1`/`$2` positional args all depend on the trigger source and the distro's hook-runner version.

These are recipes the user adapts, not turnkey installers. dddns-side tooling to auto-install any of them is explicitly out of scope (see §"What dddns does not do").

## Context

Polling (`*/30 * * * *`) has up to 30 minutes of latency between IP change and DNS update. Most home ISPs don't change IPs often enough for this to matter, but for users with frequent changes (PPPoE disconnects, mobile/4G/5G uplinks) event-driven detection cuts latency to seconds.

On UniFi OS, the router's built-in inadyn pushes to `dddns serve`. On everything else, the platform itself must invoke `dddns update` when the interface changes.

## Options by platform

### Raspberry Pi / generic Debian with dhcpcd

dhcpcd fires exit hooks on lease changes. Drop a script in `/lib/dhcpcd/dhcpcd-hooks/90-dddns`:

```bash
#!/bin/sh
case "$reason" in
    BOUND|RENEW|REBIND|REBOOT|STATIC)
        if [ "$interface" = "eth0" ] || [ "$interface" = "wlan0" ]; then
            /usr/local/bin/dddns update --quiet &
        fi
        ;;
esac
```

Reliable, stdlib-only, no daemon. The canonical recommendation for Pi.

### Generic Linux with NetworkManager

`/etc/NetworkManager/dispatcher.d/90-dddns`:

```bash
#!/bin/sh
[ "$1" = "eth0" ] || exit 0
case "$2" in
    up|dhcp4-change|dhcp6-change) /usr/local/bin/dddns update --quiet & ;;
esac
```

Covers most desktop Linux distributions.

### OpenWrt

`/etc/hotplug.d/iface/90-dddns`:

```sh
[ "$ACTION" = "ifup" ] || exit 0
[ "$INTERFACE" = "wan" ] || exit 0
/usr/local/bin/dddns update --quiet &
```

Documented as a user-side recipe.

### pfSense / OPNsense

Both have a "Services → Dynamic DNS" feature that can push to a generic dyndns v2 endpoint. Users point the built-in client at a `dddns serve` running on a helper device (Pi) on the LAN. This reuses the UniFi bridge architecture without any pfSense-specific code in dddns.

### macOS

`launchd` has no direct network-change trigger. The `SystemConfiguration` framework's `SCDynamicStore` does, but wrapping it needs cgo or a helper daemon — both break the single static-binary model.

**Recommendation:** cron stays the default. Users who want faster detection wire up a launch agent polling `networksetup -getinfo` or reading the SCF via a shell helper.

### Windows

WMI event subscriptions (`__InstanceModificationEvent` on `Win32_NetworkAdapterConfiguration`) need a long-running process.

**Recommendation:** Task Scheduler every 5 minutes stays the default. No WMI integration planned.

## What dddns does not do

**No auto-install of hooks.** A `dddns install-hooks` subcommand would have to know about each platform's init system, quote-correctness of its shell scripts, and the user's WAN interface name. All three are fragile. Hooks are a documented recipe; the user installs them.

**No platform-auto-detect watcher daemon.** A long-lived `dddns watch-wan` process picking the best event source per platform would add persistent-process lifecycle, platform-specific code (netlink on Linux, SCF on macOS, WMI on Windows), and a fallback config surface. The cost outweighs the ~30-minute latency saving for most users.

## Summary

| Platform | Recommendation |
|---|---|
| UniFi UDR/UDM | Serve mode (shipped) |
| Raspberry Pi | Cron default; dhcpcd hook for instant detection |
| Generic Linux | Cron default; NetworkManager dispatcher script for instant detection |
| OpenWrt | Cron default; hotplug.d script for instant detection |
| pfSense / OPNsense | Built-in dyndns client → `dddns serve` on LAN helper |
| macOS / Windows | Cron (Task Scheduler on Windows) |
