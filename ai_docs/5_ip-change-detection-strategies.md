# IP Change Detection Strategies

## Overview

Current dddns uses polling (cron every 30 minutes) to detect IP changes. This document explores more efficient, event-driven approaches, especially for gateway devices that can detect WAN interface changes immediately.

## Current Approach: Polling

### How It Works
```bash
*/30 * * * * /usr/local/bin/dddns update --quiet
```

**Pros:**
- Simple and universal
- Works on any platform
- No special permissions needed

**Cons:**
- Up to 30-minute delay in detecting changes
- Unnecessary API calls when IP is stable
- Wastes resources checking unchanged IPs

## Platform-Specific Event-Driven Approaches

### 1. Ubiquiti UniFi OS (UDM/UDR/USG)

#### Method A: Monitor WAN Interface Events

UniFi OS provides several hooks for detecting WAN changes:

```bash
# Monitor dhclient hooks
/etc/dhcp/dhclient-exit-hooks.d/dddns
```

**Implementation:**
```bash
#!/bin/sh
# /etc/dhcp/dhclient-exit-hooks.d/dddns
# Triggered when DHCP lease changes

case "$reason" in
    BOUND|RENEW|REBIND|REBOOT)
        # New IP obtained or renewed
        if [ "$interface" = "eth8" ] || [ "$interface" = "eth9" ]; then
            # eth8/eth9 are typical WAN interfaces on UDM
            logger -t dddns "WAN IP change detected on $interface"
            /usr/local/bin/dddns update --quiet &
        fi
        ;;
esac
```

#### Method B: PPPoE Hook (if using PPPoE)

```bash
# /etc/ppp/ip-up.d/dddns
#!/bin/sh
# Triggered when PPPoE connection established

if [ "$PPP_IFACE" = "ppp0" ]; then
    logger -t dddns "PPPoE IP change: $PPP_LOCAL"
    /usr/local/bin/dddns update --quiet &
fi
```

#### Method C: Monitor UniFi Events via unifi-os API

```go
// internal/watchers/unifi/watcher.go
package unifi

import (
    "bufio"
    "os/exec"
    "strings"
)

func WatchWANEvents() {
    // Monitor UniFi OS controller events
    cmd := exec.Command("ubnt-tools", "eventstream")
    stdout, _ := cmd.StdoutPipe()
    cmd.Start()

    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
        line := scanner.Text()
        if strings.Contains(line, "wan.status") ||
           strings.Contains(line, "wan.ip_address") {
            // WAN change detected
            triggerUpdate()
        }
    }
}
```

#### Method D: NetworkManager Dispatcher (newer UniFi OS)

```bash
# /etc/NetworkManager/dispatcher.d/90-dddns
#!/bin/sh

if [ "$1" = "eth8" ] || [ "$1" = "eth9" ]; then
    case "$2" in
        up|dhcp4-change|dhcp6-change)
            /usr/local/bin/dddns update --quiet &
            ;;
    esac
fi
```

### 2. Linux Systems (General)

#### Method A: Netlink Socket Monitoring

```go
// internal/watchers/linux/netlink.go
package linux

import (
    "github.com/vishvananda/netlink"
    "log"
)

func WatchNetlinkEvents() error {
    // Subscribe to route and address updates
    ch := make(chan netlink.AddrUpdate)
    done := make(chan struct{})

    if err := netlink.AddrSubscribe(ch, done); err != nil {
        return err
    }

    go func() {
        for update := range ch {
            // Filter for WAN interface
            if isWANInterface(update.LinkIndex) {
                if update.NewAddr {
                    log.Printf("New IP detected: %s", update.LinkAddress.IP)
                    triggerUpdate()
                }
            }
        }
    }()

    return nil
}
```

#### Method B: inotify on /sys/class/net

```go
// Monitor network interface changes
func WatchSysNet() error {
    watcher, _ := fsnotify.NewWatcher()

    // Watch WAN interface carrier and operstate
    watcher.Add("/sys/class/net/eth0/carrier")
    watcher.Add("/sys/class/net/eth0/operstate")

    go func() {
        for event := range watcher.Events {
            if event.Op&fsnotify.Write == fsnotify.Write {
                checkIPChange()
            }
        }
    }()

    return nil
}
```

#### Method C: NetworkManager D-Bus

```go
// internal/watchers/linux/networkmanager.go
package linux

import "github.com/godbus/dbus/v5"

func WatchNetworkManager() error {
    conn, err := dbus.SystemBus()
    if err != nil {
        return err
    }

    // Subscribe to NetworkManager signals
    conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
        "type='signal',interface='org.freedesktop.NetworkManager'")

    ch := make(chan *dbus.Signal, 10)
    conn.Signal(ch)

    go func() {
        for signal := range ch {
            if signal.Name == "org.freedesktop.NetworkManager.StateChanged" {
                // Network state changed
                triggerUpdate()
            }
        }
    }()

    return nil
}
```

### 3. OpenWrt/DD-WRT

#### Hotplug Scripts

```bash
# /etc/hotplug.d/iface/90-dddns
#!/bin/sh

[ "$ACTION" = "ifup" ] || [ "$ACTION" = "ifupdate" ] || exit 0
[ "$INTERFACE" = "wan" ] || exit 0

logger -t dddns "WAN interface $ACTION detected"
/usr/local/bin/dddns update --quiet &
```

### 4. pfSense/OPNsense

#### Event Hook

```php
# /usr/local/etc/rc.d/dddns_hook
#!/usr/local/bin/php
<?php
// Hook into pfSense events
$subsystem = "event";
$action = "interface_configure";

if ($argv[1] == "wan") {
    exec("/usr/local/bin/dddns update --quiet &");
}
?>
```

### 5. Windows

#### WMI Event Monitoring

```go
// internal/watchers/windows/wmi.go
package windows

import (
    "github.com/StackExchange/wmi"
)

func WatchWMIEvents() error {
    // Monitor Win32_NetworkAdapterConfiguration changes
    query := `SELECT * FROM __InstanceModificationEvent WITHIN 2
              WHERE TargetInstance ISA 'Win32_NetworkAdapterConfiguration'
              AND TargetInstance.IPEnabled = TRUE`

    // Set up WMI event listener
    // ... WMI implementation

    return nil
}
```

### 6. macOS

#### System Configuration Framework

```go
// internal/watchers/darwin/scf.go
package darwin

/*
#cgo LDFLAGS: -framework SystemConfiguration
#include <SystemConfiguration/SystemConfiguration.h>

void networkChangeCallback(SCDynamicStoreRef store, CFArrayRef changedKeys, void *info);
*/
import "C"

func WatchSystemConfiguration() error {
    // Monitor System Configuration for network changes
    // Uses SCDynamicStore to watch for IP changes

    return nil
}
```

## Hybrid Approach: Smart Detection

### Proposed Implementation

```go
// internal/watchers/watcher.go
package watchers

import (
    "runtime"
    "time"
)

type IPWatcher interface {
    Start() error
    Stop() error
    OnChange(callback func(newIP string))
}

func NewWatcher() IPWatcher {
    switch runtime.GOOS {
    case "linux":
        if isUniFiOS() {
            return &UniFiWatcher{}
        }
        if hasNetlink() {
            return &NetlinkWatcher{}
        }
        return &PollWatcher{Interval: 5 * time.Minute}

    case "darwin":
        return &SCFWatcher{}

    case "windows":
        return &WMIWatcher{}

    default:
        return &PollWatcher{Interval: 30 * time.Minute}
    }
}
```

### Configuration Options

```yaml
# config.yaml
ip_detection:
  # Method: auto, poll, netlink, unifi, dhcp
  method: auto

  # Poll settings (if using poll or as fallback)
  poll_interval: 30m

  # Interface to monitor (if using interface watching)
  watch_interface: eth0

  # Enable fallback to polling if event detection fails
  fallback_to_poll: true
  poll_fallback_interval: 60m
```

## Implementation Strategy for dddns

### Phase 1: Daemon Mode (Optional)

Add optional daemon mode that can use event-driven detection:

```bash
# Traditional cron mode (current)
*/30 * * * * dddns update --quiet

# New daemon mode with event detection
dddns daemon --watch-wan
```

### Phase 2: Platform Detection

```go
// internal/platform/detector.go
func DetectPlatform() Platform {
    // Check for UniFi OS
    if fileExists("/usr/bin/ubnt-tools") {
        return UniFiOS
    }

    // Check for OpenWrt
    if fileExists("/etc/openwrt_release") {
        return OpenWrt
    }

    // Check for pfSense
    if fileExists("/etc/platform") {
        content, _ := os.ReadFile("/etc/platform")
        if strings.Contains(string(content), "pfSense") {
            return PfSense
        }
    }

    return Generic
}
```

### Phase 3: Hook Installation

```go
// cmd/install_hooks.go
var installHooksCmd = &cobra.Command{
    Use:   "install-hooks",
    Short: "Install platform-specific IP change hooks",
    RunE: func(cmd *cobra.Command, args []string) error {
        platform := platform.DetectPlatform()

        switch platform {
        case platform.UniFiOS:
            return installUniFiHooks()
        case platform.OpenWrt:
            return installOpenWrtHooks()
        default:
            return fmt.Errorf("platform does not support hooks, use cron mode")
        }
    },
}
```

## Comparison of Approaches

| Method | Latency | CPU Usage | Complexity | Reliability | Platforms |
|--------|---------|-----------|------------|-------------|-----------|
| **Cron Polling** | 30 min | Low | Simple | High | All |
| **DHCP Hooks** | Instant | Minimal | Medium | High | Linux/Unix |
| **Netlink** | Instant | Minimal | Medium | High | Linux |
| **UniFi Events** | Instant | Minimal | Complex | Medium | UniFi OS |
| **WMI Events** | <2 sec | Low | Complex | High | Windows |
| **Daemon + Poll** | 1-5 min | Low | Simple | High | All |

## Recommendations

### For UDM/UDR Specifically

1. **Primary**: DHCP client hooks (`/etc/dhcp/dhclient-exit-hooks.d/`)
   - Most reliable on UniFi OS
   - Instant detection
   - Survives firmware updates if placed in `/data/`

2. **Fallback**: Short-interval polling (5 minutes)
   - As backup when hooks fail
   - Still better than 30-minute cron

### For General Deployment

1. **Keep polling as default** - Works everywhere
2. **Add optional daemon mode** - For users who want faster detection
3. **Document platform hooks** - For advanced users
4. **Auto-detect capability** - Install hooks if platform supports

## Sample Implementation for UniFi OS

### Install Script

```bash
#!/bin/sh
# install-unifi-hooks.sh

cat > /data/on_boot.d/10-dddns.sh << 'EOF'
#!/bin/sh
# Ensure dddns hook persists after firmware updates

mkdir -p /etc/dhcp/dhclient-exit-hooks.d/

cat > /etc/dhcp/dhclient-exit-hooks.d/dddns << 'HOOK'
#!/bin/sh
case "$reason" in
    BOUND|RENEW|REBIND|REBOOT)
        if [ "$interface" = "eth8" ] || [ "$interface" = "eth9" ]; then
            logger -t dddns "WAN IP change detected"
            /data/dddns/dddns update --quiet &
        fi
        ;;
esac
HOOK

chmod +x /etc/dhcp/dhclient-exit-hooks.d/dddns
EOF

chmod +x /data/on_boot.d/10-dddns.sh
/data/on_boot.d/10-dddns.sh

echo "✓ UniFi OS hooks installed"
echo "✓ Will detect WAN IP changes instantly"
echo "✓ Survives firmware updates"
```

## Benefits of Event-Driven Detection

1. **Instant Updates**: DNS updated within seconds of IP change
2. **Reduced API Calls**: Only update when actually needed
3. **Lower Resource Usage**: No periodic polling
4. **Better for Dynamic IPs**: Catch short-lived IP changes
5. **Professional**: More elegant than polling

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Hooks fail silently | Keep cron as backup (hourly) |
| Platform differences | Detect platform, use appropriate method |
| Complex implementation | Keep polling as default option |
| Firmware updates remove hooks | Use persistent storage (/data on UniFi) |

## Conclusion

While polling works universally, platform-specific event detection can provide:
- **Instant detection** (seconds vs 30 minutes)
- **Lower resource usage** (event-driven vs periodic)
- **Fewer API calls** (only on actual changes)

For UniFi OS specifically, DHCP hooks are the best approach, providing instant detection while maintaining simplicity. The implementation should be optional, with polling remaining the default for maximum compatibility.