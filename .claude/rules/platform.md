# Platform & Configuration

Applies to: runtime environment + on-device config.

## Platform Profiles

| Platform | Config Path | Cache Path | Device ID Source |
|----------|-------------|------------|------------------|
| UDM/UDR  | `/data/.dddns/` | `/data/.dddns/last-ip.txt` | `/proc/ubnthal/system.info` |
| Linux    | `~/.dddns/` | `~/.dddns/last-ip.txt` | `/sys/class/net/eth0/address` |
| macOS    | `~/.dddns/` | `~/.dddns/last-ip.txt` | `system_profiler` Hardware UUID |
| Windows  | `%APPDATA%/dddns/` | `%APPDATA%/dddns/last-ip.txt` | `wmic` Machine GUID |
| Docker   | `/config/` | `/config/last-ip.txt` | Container ID from `/proc/self/cgroup` |

Supported release artifacts: UDM/UDR ARM64, Linux AMD64/ARM64/ARM, macOS Intel/Apple Silicon, Windows AMD64/ARM64, Docker.

## Standard Config (`config.yaml`)

```yaml
aws_region: "us-east-1"
hosted_zone_id: "ZXXXXXXXXXXXXX"
hostname: "home.example.com"
ttl: 300
access_key: "AKIAXXXXXXXXXXXXXX"    # Optional if using AWS profile / IAM role
secret_key: "xxxxxxxxxxxxxxxx"      # Optional if using AWS profile / IAM role
```

## Secure Config (`config.secure`)

- Encrypted with device-specific key (AES-256-GCM, derived from MAC / UUID / serial).
- Cannot be moved between devices — decrypt fails, dddns falls back to plaintext `config.yaml`.
- File mode 0400 enforced at load.

## Gotchas

- **Permission denied** — `config.yaml` needs 0600; `config.secure` needs 0400.
- **Encryption fails** — device ID source is not accessible (container sandbox, restricted profile).
- **UDM persistence** — must use `/data/` paths; other paths wipe on firmware update.
- **Windows paths** — always use `filepath.Join()`; backslash literals break cross-platform tests.
