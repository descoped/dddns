# Provider Architecture (HTTP-only)

**Status:** Planned — design for unstarted work. Nothing in `main` implements this yet.
**Confidence:** Medium — direction is committed; several quantitative claims carry verification callouts. See §"Verification needed before implementing".
**Last reviewed:** 2026-04-17

## Scope

Support DNS providers beyond AWS Route53 via a thin provider interface over HTTP-only transport. Retire the AWS SDK dependency so the binary stays small enough for UDM/UDR and Raspberry Pi (<10 MB).

## Out of scope

- Which providers to ship first and in what order — see `1_provider-catalog.md`.
- Event-driven IP detection on non-UniFi platforms — see `2_non-unifi-event-detection.md`.
- Credential encryption enhancements (KDF, per-install salt, passphrase) — see `4_security-roadmap.md`.
- UniFi serve-mode internals — already shipped, see `5_unifi-ddns-bridge.md`.

## Verification needed before implementing

Claims in this doc that carry estimates or inheritance from earlier PoC sketches rather than measurements against current `main`. Treat them as credible-estimate, not measured-fact, until validated by a spike.

- **SigV4 ≈ 100 lines** (§Route53 port) — inherited from the earlier PoC writeup, not measured. Confirm the real line count before using it as a sizing input.
- **~8 MB vs. ~25 MB binary** (§Why HTTP-only) — estimated. Step 1 of the implementation sequence (§Implementation sequence) should measure the current `dddns_Linux_arm64` size on `main`, then measure again after the SDK removal. Confirm the <10 MB target holds.
- **IAM compatibility with hand-signed SigV4** (§Route53 port) — the scoped policy in `docs/aws-setup.md` was validated with SDK-signed calls. Condition keys *should* match hand-signed requests since they key off request shape, not the signer — but this has not been smoke-tested. Run one real UPSERT via a spike against the policy before merging the port.
- **`targets:` schema composability** (§Multi-target) — new shape, not prototyped against the current `viper`-backed config loader. Build a parse-and-validate prototype (no update flow) first to verify it composes cleanly with `server:` and `ip_source`, and to confirm the single-provider → multi-target detection heuristic behaves as described.

These are spike targets, not blockers — the design direction holds. The first implementation PR should record the measured values in this doc.

## Decisions

1. **HTTP-only, no vendor SDKs.** Every provider uses `net/http` + stdlib. AWS Route53 is ported to hand-signed SigV4 requests; `aws-sdk-go-v2` is removed from `go.mod`.
2. **Minimal client interface.** The shipped `internal/updater.DNSClient` (2-method) stays the core contract. Provider selection, validation, and metadata live on a `Provider` record next to it, not as extra methods on the client.
3. **Flat config stays valid.** The existing single-provider YAML shape continues to load. A new optional `targets:` map enables multi-provider setups without breaking current users.
4. **Serve mode is orthogonal.** `server:` already works alongside any provider — the listener pushes to whichever provider(s) the config names.

## Why HTTP-only

| | HTTP-only | SDK-based (current) |
|---|---|---|
| Binary size | ~8 MB | ~25 MB |
| Runtime memory | ~15 MB | ~40 MB |
| Direct `go.mod` deps | 0 | 30+ (AWS SDK tree) |
| Build time | Fast | Slow |
| UDM / Pi fit | Good | Marginal |

The trade is implementing AWS SigV4 ourselves (~100 lines — see §Route53 port). Every other provider is plain REST/JSON with Bearer or Basic auth.

## Interface

The shipped contract in `internal/updater/updater.go:72-75`:

```go
type DNSClient interface {
    GetCurrentIP(ctx context.Context) (string, error)
    UpdateIP(ctx context.Context, newIP string, dryRun bool) error
}
```

Keep as-is. Add a lightweight `Provider` record for the factory pattern:

```go
type Provider struct {
    Name         string      // "aws", "cloudflare", "domeneshop", ...
    Client       DNSClient   // the wire implementation
    Hostname     string
    TTL          int64
    Experimental bool
}

func NewProvider(name string, cfg map[string]any, vault crypto.ProviderVault) (*Provider, error)
```

- Unknown `name` → fail closed.
- Provider-specific config validation happens inside `NewProvider` — fail fast at construction rather than add a `ValidateConfig()` method on the client.
- `vault` is the decryption helper for provider-specific secrets; `DNSClient` stays free of crypto knowledge.

## Route53 port (SDK → REST)

The one non-trivial provider. Route53 speaks XML and requires AWS Signature v4.

**Endpoints used:**
- `GET  https://route53.amazonaws.com/2013-04-01/hostedzone/{zoneId}/rrset?name={fqdn}&type=A&maxitems=1`
- `POST https://route53.amazonaws.com/2013-04-01/hostedzone/{zoneId}/rrset`

**SigV4:** ~100 lines of canonical-request construction, HMAC-SHA256 key derivation, `AWS4-HMAC-SHA256` authorization header. No retry logic, no regional endpoint discovery, no credential provider chain — the shipped config supplies access/secret keys directly.

**IAM compatibility:** the scoped policy in `docs/aws-setup.md` (`route53:ChangeResourceRecordSetsNormalizedRecordNames` / `RecordTypes` / `Actions`) keys off the request shape, not the signer. SigV4-signed requests hit the same condition keys. Smoke-test against a real zone before merging.

**Clock skew:** SigV4 rejects requests with timestamps more than 15 minutes off AWS's clock. UDM/UDR and most Pi images run NTP by default. Document as a prerequisite rather than trying to compensate.

**XML:** `encoding/xml` from stdlib. Define only the fields we read (Name, Type, Value); no need for the full Route53 XSD.

## Package layout

```
internal/providers/
├── providers.go               # Provider type, factory registry, DNSClient re-export
├── aws/
│   ├── route53.go             # REST client + SigV4 signer
│   └── route53_test.go
├── cloudflare/
│   ├── cloudflare.go          # REST client + Bearer auth
│   └── cloudflare_test.go
└── ...
```

Each provider registers in its `init()`. Tests use `httptest.NewServer` — no live API calls in CI.

`internal/dns/route53.go` (the current AWS-SDK-based client) becomes `internal/providers/aws/route53.go` and swaps its imports from `aws-sdk-go-v2/service/route53` to stdlib + the new SigV4 signer.

## Config schema

### Single-provider (current, preserved)

```yaml
aws_region: us-east-1
hosted_zone_id: Z...
hostname: home.example.com
ttl: 300
aws_access_key: AKIA...
aws_secret_key: ...
ip_source: auto
```

Internally interpreted as a single implicit target named `default` with `provider: aws`. No user action required.

### Multi-target (opt-in)

```yaml
ip_source: auto

targets:
  home-aws:
    provider: aws
    aws_region: us-east-1
    hosted_zone_id: Z...
    hostname: home.example.com
    ttl: 300
    aws_access_key: AKIA...
    aws_secret_key: ...

  home-cloudflare:
    provider: cloudflare
    zone_id: abc...
    hostname: home.example.com
    ttl: 300
    api_token: ...

default_targets:
  - home-aws
  - home-cloudflare

server:
  # ... unchanged; applies across all targets ...
```

`dddns update` with no `--target` uses `default_targets`. `dddns update --target home-aws` picks one. `dddns update --all` hits every target.

### Secure config

Each target's credentials encrypt to a per-target `credentials_vault` field. The `SecretVault` pattern already used for `ServerConfig` (`internal/config/secure_config.go`) extends naturally — the derivation includes the target name so stealing one target's vault doesn't compromise another.

### Migration

Detection is shape-based. No `version:` marker in the file:

- Top-level AWS fields present, no `targets:` → single-provider (current behavior).
- `targets:` present → multi-target.
- Both present → validation error (ambiguous).

For a user wanting to move from flat → multi-target, `dddns config add-target --name X --provider Y` converts in-place: wraps the existing flat fields into `targets.default` and appends the new target.

## Command surface

Existing commands unchanged for flat configs. New flags are additive.

```
dddns update                                 # default_targets
dddns update --target home-aws               # one target
dddns update --target home-aws --target home-cloudflare
dddns update --all                           # every configured target
dddns config add-target --name X --provider Y
dddns config list-targets
dddns secure enable [--target X]             # per-target vault encryption
```

**Parallel behavior:** multiple targets update concurrently via `sync.WaitGroup` with a 30-second context per target. Failure in one target does not abort others. The exit code is non-zero if any target failed.

## Implementation sequence

Each step lands independently with its own tests.

1. **Port Route53 from SDK to REST.** Single provider, no interface change yet. `internal/dns/route53.go` keeps its name and callers; imports swap from AWS SDK to stdlib + SigV4. Tests use `httptest`. `aws-sdk-go-v2` removed from `go.mod`. Binary size check: `dddns_Linux_arm64` drops below 10 MB.

2. **Extract the provider package.** `internal/providers/` with `Provider`, `NewProvider`, registry. Route53 moves from `internal/dns/` to `internal/providers/aws/`. `internal/updater` imports `internal/providers`. Behavior identical for flat configs.

3. **Multi-target config handling.** Flat-config path unchanged. Parse `targets:`. `dddns update --target` / `--all` plumbing. Per-target cache keys (the current single cache file splits into one per target — prevents cross-provider contamination).

4. **First non-AWS provider.** Cloudflare is the natural pick: largest free tier, cleanest API, JSON only. Validates the interface on a non-AWS provider before the refactor calcifies.

5. **Per-target vault encryption.** Extend `SecretVault` from `ServerConfig` to per-target credentials. `dddns secure enable --target X` re-encrypts a specific target without touching the others.

No single mega-refactor. Steps 1–2 ship before any new provider arrives.

## Non-goals

- Retry with exponential backoff. Fail-fast per the project's "no complex retry logic" rule.
- Connection pooling across providers. Targets run sequentially per update invocation, or in parallel via `sync.WaitGroup`; no shared HTTP connection state.
- Dispatch queue or job scheduler. Single-shot, exit.
- Provider capability discovery (AAAA, CAA, MX). A-record only, same as current scope.
- SDK coexistence. When the Route53 port lands, `aws-sdk-go-v2` is removed, not kept as a fallback.
