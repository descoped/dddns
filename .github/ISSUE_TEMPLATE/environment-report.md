---
name: Environment / platform report
about: File this when you want dddns to support your platform, or when adapting dddns to your setup is blocked on something I can't see from here.
labels: 'platform, needs-triage'
---

<!--
Purpose

Without concrete details from your box, I can only guess how dddns should
install and persist on your platform. Filling this in — even the "no" boxes —
gives me enough to decide whether dddns can just-work for you, or needs a
small adapter, or falls outside scope.

Ground rules

1. NEVER paste the contents of config.yaml or config.secure. They hold
   your AWS credentials. The probes below never read those files.
2. NEVER paste the full value of shared secrets, API tokens, or device IDs
   you consider sensitive. If a probe returns something you'd rather not
   share, redact it with XXXX.
3. The probes below are READ-ONLY. They do not install or modify anything.
4. Run each block on the target machine, then paste the FULL output
   under the corresponding heading. Raw output is more useful than your
   interpretation.

If any probe fails because a command is missing, leave its output block
in place with the error — that absence IS the signal I need.
-->

## 1. What you want

- Platform / distro / device (one line):
- Run mode preference: [ ] cron (polling)  [ ] serve (UniFi-only, event-driven)  [ ] whatever works
- Should dddns survive reboots? [ ] yes  [ ] no (run-once is fine)
- Should dddns survive firmware upgrades (UniFi)? [ ] yes  [ ] no  [ ] N/A

## 2. System identity

```bash
echo "=== UniFi identity (empty if not UniFi) ==="
cat /proc/ubnthal/system.info 2>/dev/null | head -20
echo "=== os-release ==="
cat /etc/os-release 2>/dev/null
echo "=== uname ==="
uname -a
echo "=== arch ==="
uname -m
```

<details><summary>Paste output here</summary>

```
```

</details>

## 3. Scheduler availability

```bash
echo "=== systemd ===" && command -v systemctl && systemctl --version 2>/dev/null | head -1
echo "=== cron ===" && command -v crontab; ls -d /etc/cron.d 2>/dev/null
echo "=== launchd (macOS) ===" && command -v launchctl 2>/dev/null
echo "=== logger (for journald routing) ===" && command -v logger
echo "=== running in container? ===" && ls /.dockerenv 2>/dev/null; ls /run/.containerenv 2>/dev/null
```

<details><summary>Paste output here</summary>

```
```

</details>

## 4. Persistence hooks (UniFi / on-boot)

```bash
echo "=== on_boot.d ===" && ls -la /data/on_boot.d/ 2>/dev/null || echo "(missing)"
echo "=== udm-boot.service ==="
systemctl is-active udm-boot.service 2>/dev/null
systemctl status udm-boot.service --no-pager 2>&1 | head -10
echo "=== /data writable ===" && ls -ld /data 2>/dev/null; df -h /data 2>/dev/null
echo "=== /etc writable probe ==="
touch /etc/.dddns-probe 2>&1 && rm -f /etc/.dddns-probe && echo "writable" || echo "read-only"
```

<details><summary>Paste output here</summary>

```
```

</details>

## 5. Network shape

```bash
echo "=== default routes (main table) ===" && ip -4 route show default 2>/dev/null
echo "=== all IPv4 addresses ==="
ip -4 addr 2>/dev/null | awk '/^[0-9]+:/{iface=$2} /inet /{print iface,$2}' | sed 's/://g'
echo "=== policy rules (non-empty on routers with PBR) ==="
ip rule list 2>/dev/null
echo "=== routes across all tables ==="
ip -4 route show table all 2>&1 | grep -v -E "^(local|broadcast)" | head -30
echo "=== /proc/net/route (what wanip reads) ==="
head -10 /proc/net/route 2>/dev/null
echo "=== netns ===" && ip netns list 2>/dev/null
```

<details><summary>Paste output here</summary>

```
```

</details>

## 6. Existing dddns install (skip if fresh machine)

```bash
echo "=== binary / symlink ==="
ls -la /data/dddns/ /usr/local/bin/dddns /opt/dddns/ 2>/dev/null
echo "=== cron entry ==="
ls -la /etc/cron.d/dddns 2>/dev/null; crontab -l 2>/dev/null | grep -i dddns
echo "=== systemd unit ==="
systemctl status dddns.service --no-pager 2>&1 | head -6
echo "=== launchd agent (macOS) ==="
launchctl list 2>/dev/null | grep -i dddns
echo "=== boot script (UniFi) ==="
ls -la /data/on_boot.d/20-dddns.sh 2>/dev/null
echo "=== version + sanity ==="
dddns --version 2>/dev/null
dddns config check 2>&1 | tail -10
echo "=== last update log (redact if needed) ==="
tail -5 /var/log/dddns.log 2>/dev/null
```

<details><summary>Paste output here</summary>

```
```

</details>

## 7. Anything unusual

Call out anything special about your setup that the probes above might not
capture. A few one-liners, no essay.

- ISP / WAN shape (native dual-stack, CGNAT, PPPoE, ISP CPE in bridge mode, static, etc.):
- IPv6 story (only, dual-stack, off):
- Split DNS / firewall rules that could block loopback `127.0.0.1:53353`:
- VPN always-on that routes outbound HTTP:
- Air-gapped / no GitHub reachability:
- Read-only root filesystem:
- Non-root-only constraints (no sudo, container rootless, etc.):
- Anything you already tried and what happened:

## 8. What you need from me

- [ ] Add platform support (new mode / scheduler / path layout)
- [ ] Fix a crash / incorrect behaviour on my platform (attach output above)
- [ ] Advice on how to run dddns on this system with the current release
- [ ] Something else:

<!--
Thanks. I read these in full. If a probe returned nothing where output was
expected, don't worry about it — the absence is what I need. If you redacted
anything, say so and what was in its place, so I don't misread a missing
field as "not present".
-->
