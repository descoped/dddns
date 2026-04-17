# Implementation Specification: UniFi-to-Route 53 Dynamic DNS Bridge

## 1. Executive Summary

This document specifies the architecture and implementation requirements for a custom Go-based Dynamic DNS (DDNS) bridge running natively on a UniFi Dream Router (UDR7). The goal is to replace a polling-based "cron" mechanism with an event-driven, local-only HTTP listener that integrates directly with the UniFi OS Network Controller UI to manage AWS Route 53 records.

## 2. Sequence Flow

The following sequence describes the interaction between the UniFi OS, the custom Go binary, and the AWS Route 53 API:

1. **System Boot:** The UDR7 executes the boot script located in `/data/on_boot.d/`. This script initializes environment variables (AWS credentials) and launches the Go binary in the background.
2. **IP Change Detection:** The UniFi OS detects a public IP change on the WAN interface (e.g., via DHCP lease renewal).
3. **Local Trigger:** The UniFi `inadyn` client issues a local HTTP GET request to `127.0.0.1:[PORT]`.
4. **Verification & Processing:** The Go binary parses the request parameters (`hostname` and `myip`).
5. **Upstream Update:** The Go binary executes a `ChangeResourceRecordSets` call via the AWS SDK v2 for Go.
6. **Handshake:** The Go binary returns a status string (e.g., `good`) to the UniFi client.
7. **UI Update:** The UniFi Network Controller reflects a "Normal" status, and the dynamic IP warning is cleared.

## 3. Technical Requirements

### 3.1 Security & Networking

- **Binding:** The HTTP server MUST bind exclusively to `127.0.0.1` (localhost). This ensures the service is inaccessible from the WAN, LAN, and WLAN, creating a zero-attack surface for external actors.
- **Port Management:** A non-privileged port above 1024 should be used (e.g., `8080`).
- **Protocol:** Standard HTTP is utilized as the traffic remains internal to the router's process space.

### 3.2 Go Binary Implementation

- **HTTP Handler:** Implement a handler to parse query strings provided by the UniFi Controller.
- **AWS SDK Integration:** Utilize `aws-sdk-go-v2/service/route53` for record management.
- **Response Codes:**
  - `good`: Update successful.
  - `nochg`: IP matches existing record (no API call required).
  - `911` / `abuse`: Fatal error or throttling detected.

### 3.3 Persistence (UDR7 Specifics)

- **Binary Path:** `/data/custom/dddns/dddns`
- **Persistence:** UniFi OS firmware updates wipe the root filesystem. All logic must reside in `/data/` to survive updates.
- **Boot Hook:** Use the `on-boot-script` utility to ensure the daemon starts automatically after reboots or firmware upgrades.

## 4. UI Configuration Reference

To link the Go binary to the UniFi Network Controller, the "Dynamic DNS" settings should be configured as follows:

| Field     | Value                                              |
|-----------|----------------------------------------------------|
| Service   | Custom                                             |
| Hostname  | `[your-domain.no]`                                 |
| Username  | `internal` (placeholder)                           |
| Password  | `internal` (placeholder)                           |
| Server    | `localhost:8080/update?hostname=%h&myip=%i`        |

## 5. Deployment Checklist

- [ ] Compile Go binary for `linux/arm64`.
- [ ] Transfer binary to `/data/custom/dddns/`.
- [ ] Configure AWS IAM policy with minimal permissions (`route53:ChangeResourceRecordSets` for the specific Hosted Zone).
- [ ] Create and test the `/data/on_boot.d/` trigger script.
- [ ] Verify local connectivity via `curl` inside the SSH session before configuring the UI.

---

*Document generated for private network implementation on Ubiquiti UniFi OS 4.0+ hardware.*

---

## Appendix A: Assessment

### Alignment with Existing ai_docs

The spec extends `5_ip-change-detection-strategies.md` with an approach the existing doc does not cover: use UniFi's built-in `inadyn` "Custom" provider as the event source instead of installing `dhclient` or hotplug hooks. This is elegant — it piggybacks on UI-visible status and avoids touching `/etc/`, so nothing needs reinstalling after firmware updates.

### Conflicts That Must Be Resolved Before Building

1. **SDK vs HTTP-only (contradicts docs #3 and #4).** The spec mandates `aws-sdk-go-v2/service/route53`. Docs 3 and 4 lay out the migration *away* from the SDK to raw HTTP/REST to keep the binary <10MB and avoid ~80MB of dependencies. Pick one lane — either update this spec to "signed REST requests per doc #4" or explicitly revert the HTTP-only direction.
2. **"No daemon/service mode" (CLAUDE.md).** This is explicitly listed as something not to add without explicit ask. A long-running HTTP listener is exactly that. Needs an explicit scope decision before coding.
3. **Credential handling regression.** The spec puts AWS credentials in `on_boot.d/` environment variables (plaintext shell script). Current dddns uses device-encrypted `config.secure`. The spec should reuse the existing crypto path, not introduce a weaker one.

### Critical Gaps in the Spec

- **No input validation.** `myip` must be parsed and rejected if RFC1918 / loopback / link-local; otherwise a misconfigured `inadyn` can publish `10.x.x.x` to public DNS.
- **No hostname authorization.** The handler must verify `hostname` matches the configured zone; otherwise the UI field becomes an unintended write primitive.
- **Incomplete dyndns protocol.** Only 3 return codes listed. `inadyn` also interprets `badauth`, `notfqdn`, `nohost`, `badagent`, `dnserr` — without them, error paths are ambiguous and UI status is wrong. Also missing: `GET`-only method guard, trailing-newline format, always-200 HTTP status convention.
- **No dedup / no local cache hookup.** The spec says "return `nochg` if IP matches" but does not specify the source of truth. Must reuse the existing `/data/.dddns/last-ip.txt` to avoid a `ListResourceRecordSets` call per request.
- **No concurrency guard.** Flapping DHCP can fire `inadyn` twice. Needs a mutex or single-flight around the Route53 call.
- **No supervision / logging path.** "Background process" with no respawn and no log file means silent failure on a headless router.
- **Defense in depth.** Loopback bind is good, but any local process can hit it. Either require the Basic-Auth credential inadyn sends to match a shared secret, or assert `RemoteAddr` explicitly — cheap insurance.

### Open Questions to Verify on Hardware Before Coding

- Does UDR7 on UniFi OS 4.0+ ship `on_boot.d` natively, or is it still the community `on-boot-script` package? The path has shifted across OS versions.
- Does the UI "Custom DDNS" `Server` field on OS 4.0 accept `host:port/path?q=%h`, or does it strip the port/path? Known to vary between versions.

### Recommendation

Decide the two scope questions first (SDK-vs-HTTP, daemon-mode policy). If both are green-lit, the spec is a solid skeleton but needs a v2 that folds in: input validation, hostname allowlist, full dyndns return-code table, reuse of existing cache + crypto, and a concurrency/logging section.

