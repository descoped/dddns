# Raspberry Pi Support

## Scope

Install and operate dddns on Raspberry Pi 2/3/4/5/Zero. Target the Pi's constrained CPU, RAM, and SD card write budget.

## Out of scope

- UniFi OS devices — see `5_unifi-ddns-bridge.md`.
- Generic Linux install — the Pi guide works for any Debian-based system; platform-specific event hooks live in `2_non-unifi-event-detection.md`.
- GPIO/HAT integrations, mDNS discovery, cluster failover. Outside dddns's single-purpose scope.

## Compatibility matrix

| Model | Arch | Binary | Resident RAM |
|---|---|---|---|
| Pi 5 | ARM64 | `dddns_Linux_arm64` | ~12 MB |
| Pi 4 | ARM64 / ARMv7 | `dddns_Linux_arm64` or `_armv7` | ~12 MB |
| Pi 3B+ | ARM64 / ARMv7 | same | ~12 MB |
| Pi 3, 2 | ARMv7 | `dddns_Linux_armv7` | ~10 MB |
| Pi Zero 2 W | ARM64 | `dddns_Linux_arm64` | ~12 MB |
| Pi Zero W | ARMv6 | `dddns_Linux_armv7` | Works; not CI-tested |

## Install

Binary tarball:

```bash
ARCH=$(uname -m)
case $ARCH in
    aarch64|arm64) BIN="dddns_Linux_arm64.tar.gz" ;;
    armv7l)        BIN="dddns_Linux_armv7.tar.gz" ;;
    *) echo "unsupported: $ARCH"; exit 1 ;;
esac

wget "https://github.com/descoped/dddns/releases/latest/download/$BIN"
tar -xzf "$BIN"
sudo install -m 755 dddns /usr/local/bin/
```

Debian packages (`dddns_arm64.deb`, `dddns_armhf.deb`) are also published per release.

## Configuration

`~/.dddns/config.yaml`:

```yaml
aws_region: us-east-1
hosted_zone_id: ZXXXXXXXXXXXXX
hostname: home.example.com
ttl: 300
aws_access_key: AKIAXXXXXXXXXXXXXX
aws_secret_key: xxxxxxxxxxxxxxxxxxxxxxxx

# Optional: reduce SD-card wear by caching the last IP in tmpfs
ip_cache_file: /run/user/1000/dddns-last-ip.txt
```

`chmod 600 ~/.dddns/config.yaml`. Run `dddns secure enable` to encrypt credentials at rest with a device-bound key.

## Automatic updates

### Cron (default)

```bash
crontab -e
*/30 * * * * /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
```

### systemd timer

Use this for journald integration.

`/etc/systemd/system/dddns.service`:

```ini
[Unit]
Description=Dynamic DNS updater
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=pi
ExecStart=/usr/local/bin/dddns update --quiet
```

`/etc/systemd/system/dddns.timer`:

```ini
[Unit]
Description=Run dddns every 30 minutes

[Timer]
OnBootSec=5min
OnUnitActiveSec=30min

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl enable --now dddns.timer
```

### Instant updates via dhcpcd hook

For PPPoE or frequently changing IPs, see the dhcpcd hook pattern in `2_non-unifi-event-detection.md`.

### Serve mode via a local dyndns client

`dddns serve` listens for dyndns v2 requests on `127.0.0.1:53353`. If your Pi runs a router-like setup (OpenWrt-on-Pi, IoT gateway with PPPoE), you can point its built-in dynamic DNS client at `http://127.0.0.1:53353/nic/update`. The same protocol documented for UniFi in `5_unifi-ddns-bridge.md` works for any dyndns v2 client.

## SD card wear

dddns writes two files: the IP cache (one line, on every update) and the log (append-only). To minimize SD writes:

- Set `ip_cache_file: /run/user/1000/dddns-last-ip.txt` (tmpfs — cleared on reboot, rebuilt on first run).
- Log via journald (systemd service) rather than to a file on the SD card.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `binary not found` | Wrong arch — `uname -m` must match the downloaded tarball. |
| `permission denied` | `sudo chmod +x /usr/local/bin/dddns` and `chmod 600 ~/.dddns/config.yaml`. |
| `network not ready` at boot | Cron delay: `@reboot sleep 60 && /usr/local/bin/dddns update --quiet`. |
| `cannot read config` | Ownership: `sudo chown $USER:$USER ~/.dddns/*`. |

## Footprint

| | dddns | ddclient | inadyn |
|---|---|---|---|
| Binary | 8 MB | N/A (Perl) | 200 KB |
| Resident RAM | 10–12 MB | 40–50 MB | 5–8 MB |
| Dependencies | none | Perl + CPAN modules | OpenSSL |
