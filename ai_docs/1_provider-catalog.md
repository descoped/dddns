# Provider Catalog

## Scope

Which DNS providers to prioritize adding to dddns once the provider architecture is in place, and what to require of each implementation.

## Out of scope

- The provider interface and HTTP-only transport — see `0_provider-architecture.md`.
- How users switch between providers or run multiple in parallel — see `0_provider-architecture.md`.

## Priority

### Tier 1 — high user demand

| Provider | Auth | Format | API doc |
|---|---|---|---|
| Cloudflare | Bearer (scoped API token) | JSON | https://developers.cloudflare.com/api/ |
| GoDaddy | API key + secret | JSON | https://developer.godaddy.com/ |
| Namecheap | API key + user + IP allowlist | XML | https://www.namecheap.com/support/api/ |

### Tier 2 — developer-focused

| Provider | Auth | Format | API doc |
|---|---|---|---|
| DigitalOcean | Bearer | JSON | https://docs.digitalocean.com/reference/api/ |
| Linode | Bearer | JSON | https://www.linode.com/api/v4/ |
| Hetzner | API token | JSON | https://dns.hetzner.com/api-docs |

### Tier 3 — niche / regional / specialized

| Provider | Auth | Format | API doc |
|---|---|---|---|
| Gandi | API key | JSON | https://doc.livedns.gandi.net/ |
| Porkbun | API key + secret | JSON | https://porkbun.com/api/json/v3/documentation |
| DuckDNS | Token in URL | Text | https://www.duckdns.org/spec.jsp |
| Domeneshop | Basic auth | JSON | https://api.domeneshop.no/docs/ |
| Name.com | Username + Token | JSON | https://www.name.com/api-docs |

## Implementation requirements

For each provider:

1. **Client in `internal/providers/<name>/`** implementing `updater.DNSClient` — see `0_provider-architecture.md`.
2. **Unit tests** with `httptest.NewServer` — no live API calls in CI.
3. **Setup doc** in `docs/providers/<name>.md` covering: how to get the API credential, minimum required permissions/scopes, rate limits.
4. **Credential vault support** via the `SecretVault` pattern — no plaintext credentials in `.secure` configs.
5. **Scoped credentials** where the provider supports it (e.g. Cloudflare token limited to one zone, Route53 IAM policy scoped to one record — `docs/aws-setup.md`).

## Acceptance criteria

A provider lands when **one** of:

1. ≥10 user requests (GitHub issues, reactions).
2. >1% of domain registrations market share (Cloudflare, GoDaddy, Namecheap meet this).
3. Fills a geographic or regulatory gap not covered by existing providers.

Plus, always:

- Stable, documented API.
- Provider has existed ≥3 years with a credible continuation plan.
- Tests runnable against a provider sandbox if one exists, otherwise against `httptest`.

## Notes per provider

**Cloudflare** — handle `proxied: true/false` explicitly. Scoped API tokens preferred over Global API Key.

**Namecheap** — client IP allowlisting is mandatory on their end; document it in the setup doc or users hit `Error 1011150`. XML parsing is stdlib only.

**GoDaddy** — OTE sandbox exists for testing. Shopper ID may be required depending on account type.

**DuckDNS** — trivial to implement (HTTP GET, token-in-URL). Worth shipping early as a smoke test for the plugin pattern since it exercises everything except credential encryption.

**Domeneshop** — well-suited to Scandinavian users; Basic Auth means the `SecretVault` must hold a `token:secret` pair rather than a single value.

## Community contributions

Open an issue with the `[provider-request]` label. Users upvote to surface demand. Contributors follow the Implementation requirements above — PRs without tests or a setup doc are held.
