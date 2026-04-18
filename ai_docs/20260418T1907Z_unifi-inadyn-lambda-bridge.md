# UniFi inadyn → Lambda bridge — implementation plan

**Status:** Planned — no code yet. Ready to implement after v0.2.0 ships.
**Confidence:** High on the architecture; medium on some IAM/OpenTofu details that get verified at `tofu plan` time.
**Last reviewed:** 2026-04-18

## Motivation

v0.2.0 validated that UniFi's built-in `inadyn` cannot reach a loopback listener because of its `-b eth4` binding. Cron mode is the UniFi-Dream supported path. Some users will still prefer **event-driven updates** from UniFi's DDNS UI — fast reaction, no polling lag. The community post will link to a parallel **AWS Lambda** receiver that inadyn *can* reach (over the public internet, which is inadyn's natural context).

This plan describes landing that receiver **inside the dddns repo** rather than spinning up a sibling project. Rationale:

- Shared ownership and release cadence (one tag covers both binary and bridge).
- One place for UniFi-specific docs.
- Go code in the Lambda reuses `internal/dns/` (SigV4 signer + Route53 REST client) — no second implementation to maintain.
- OpenTofu module lives alongside the code it deploys.

## Scope

**In scope:**

1. Go Lambda handler (new package `lambda/`) that:
   - Accepts `POST /nic/update` (or GET — match inadyn's dyndns v2 semantics).
   - Constant-time Basic Auth against a shared secret from env.
   - Ignores the `myip` query param; trusts the request source IP from API Gateway's `requestContext.identity.sourceIp` as the WAN IP to publish. *(See "Security model" — this is the L6 equivalent.)*
   - Calls `route53.UpdateIP` via `internal/dns/` (reuses the SigV4 signer dddns already ships).
   - Returns dyndns response codes (`good` / `nochg` / `badauth` / `nohost`) so inadyn parses correctly.
2. OpenTofu module (`deploy/tofu/`) defining:
   - Lambda function (Go `provided.al2023` runtime, arm64).
   - API Gateway HTTP API (cheaper than REST API, native path-based integrations).
   - IAM role scoped to `route53:ChangeResourceRecordSets` on exactly one zone + one record name (resource-level condition keys).
   - CloudWatch log group with 7-day retention.
   - Lambda reserved concurrency = 2 (cost-blast cap).
   - API Gateway throttling = 10 req/s burst 100.
3. `Makefile` target `build-lambda` that cross-compiles arm64 zip.
4. `docs/unifi-inadyn-bridge.md` covering: when to use (UniFi event-driven), how to deploy, cost, UniFi UI config, rollback.

**Out of scope:**

- Replacing dddns's Route53 client with the Lambda. dddns itself remains a standalone CLI; the Lambda is a parallel bridge.
- Multi-provider (Cloudflare etc.) in the Lambda — that's `0_provider-architecture.md` territory.
- Custom domain on the API Gateway (raises cost floor; use the auto-generated `execute-api` URL).
- WAF, Secrets Manager, DynamoDB for lockout state. API Gateway throttling + reserved concurrency handle the abuse scenarios.

## Architecture

```
UniFi UI DDNS entry
  └─► inadyn (-b eth4, binds to WAN) 
        └─► HTTPS POST to https://<id>.execute-api.<region>.amazonaws.com/prod/nic/update
              │ Authorization: Basic base64(dddns:<secret>)
              │ Query: ?hostname=<fqdn>&myip=<ignored>
              ▼
            API Gateway HTTP API (throttling, HTTPS)
              │
              ▼
            Lambda (Go, arm64, 128 MB)
              ├─► Constant-time Basic Auth check
              ├─► Validate request source IP (sourceIp from requestContext)
              ├─► internal/dns.UpdateIP (SigV4-signed ChangeResourceRecordSets)
              │     └─► Route53
              └─► Return "good <ip>" / "nochg <ip>"
```

### Why this is cost-safe even under attack

| Defense | Value |
|---|---|
| Lambda reserved concurrency | 2 — max 2 concurrent executions, everything else is throttled by Lambda |
| API Gateway throttling | 10 req/s sustained, 100 burst — AWS rejects flood before billing |
| IAM role scope | `route53:ChangeResourceRecordSets` on ONE zone + ONE record name via resource-level conditions — stolen creds can't pivot |
| CloudWatch log retention | 7 days — prevents log-storage runaway |
| No idle compute | All pay-per-invocation; dormant = $0 |

**Worst-case sustained DoS for a year:** ~315M requests → ~$65. More realistic attack (100K in a burst before you notice and rotate the secret): ~$0.02.

### Why the request-source-IP-is-the-WAN-IP decision is safe

inadyn on UniFi-Dream runs on the same host as the WAN interface. When it sends via `-b eth4`, the TCP connection's source IP IS the WAN IP. API Gateway's `requestContext.identity.sourceIp` captures that. So:

- inadyn claims `myip=1.2.3.4` via query — Lambda **ignores**, logs as `claimed`.
- TCP source at API Gateway is `<WAN-IP>` — Lambda **trusts** this as the value to publish.
- An attacker who steals the shared secret and pushes from somewhere else sees Route53 updated to *their* source IP, not the victim's — which is detectable anomaly in CloudWatch logs but not a DNS hijack against the victim's legitimate WAN.

This is the L6-equivalent defense: we never trust a claimed IP, we use the observed connection's source. Same spirit as dddns serve's "read local WAN IP via wanip.FromInterface" — different mechanism (observation) but same principle (ignore claims, trust ground truth).

## Security model — side-by-side with dddns serve

| Layer | dddns serve (loopback) | Lambda bridge |
|---|---|---|
| Transport | HTTP over `lo` | HTTPS over internet |
| Network isolation | loopback-only bind | API Gateway rate-limits + reserved concurrency |
| Auth | Basic Auth, constant-time | Basic Auth, constant-time |
| Brute force | Sliding-window lockout (5 in 60s → 5 min) | API Gateway throttle (10 req/s) |
| IP verification | Read local WAN from kernel | Use TCP source observed by API Gateway |
| IAM scope | one record via condition keys | one record via condition keys |
| Audit | JSONL file with rotation | CloudWatch logs |
| Idle attack surface | none (loopback only) | public HTTPS endpoint (but behind AWS's DDoS protection and the throttle cap) |

The Lambda bridge **opens a public HTTP surface** (that's inevitable given inadyn's constraints). We compensate by:

- No secrets reachable beyond the shared secret (no config.secure equivalent on Lambda — credentials are env-var / SSM).
- IAM scoped to the single record, so compromised Lambda role cannot pivot to other DNS.
- API Gateway's URL is unguessable (random subdomain); publishing it makes the attacker's scan irrelevant but even without publishing it, the throttle + auth hold.

## File layout

```
dddns/
├── cmd/                        # existing CLI
├── internal/                   # existing business logic
│   └── dns/                    # REUSED by lambda/ — no duplication
├── lambda/                     # NEW
│   ├── main.go                 # Lambda entry point (Go)
│   ├── handler.go              # Request parsing, auth, dispatch
│   ├── handler_test.go         # httptest-driven unit tests
│   └── README.md               # Build / deploy notes
├── deploy/tofu/                # NEW
│   ├── main.tf                 # providers, versions
│   ├── variables.tf            # zone_id, hostname, shared_secret, region
│   ├── lambda.tf               # Lambda function + log group
│   ├── apigw.tf                # HTTP API + route + integration
│   ├── iam.tf                  # role scoped to the record
│   ├── outputs.tf              # URL, test snippet
│   └── README.md               # `tofu init && tofu apply`
├── Makefile                    # ADD: build-lambda target
└── docs/
    └── unifi-inadyn-bridge.md  # NEW: user docs
```

## Implementation phases

Target each phase as a single commit / PR so the work is reviewable in slices.

### Phase 1 — Lambda handler (no infra yet)

**Goal:** Go code that works locally via `go test`, with no AWS deployment needed.

1. `lambda/main.go`: AWS Lambda handler signature `func(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)`.
2. `lambda/handler.go`: request parsing, Basic Auth check (constant-time via `crypto/subtle`), hostname validation, source-IP extraction from `event.RequestContext.HTTP.SourceIP`.
3. Route53 update via `internal/dns.NewRoute53Client` + `UpdateIP` — same code path cron mode uses.
4. `lambda/handler_test.go`: httptest-driven coverage of good auth, bad auth, wrong hostname, missing auth, source-IP extraction.

Acceptance: `go test ./lambda/...` green; `GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o /tmp/bootstrap ./lambda` produces a runnable zip binary.

### Phase 2 — OpenTofu module

**Goal:** One `tofu apply` creates the entire stack.

1. `deploy/tofu/main.tf`: provider config, AWS + random.
2. `deploy/tofu/variables.tf`: required `hosted_zone_id`, `hostname`, `shared_secret` (sensitive, no default), optional `region` (default `us-east-1`), `log_retention_days` (default 7).
3. `deploy/tofu/lambda.tf`: `aws_lambda_function` with `runtime = "provided.al2023"`, `architectures = ["arm64"]`, `memory_size = 128`, `timeout = 10`, `reserved_concurrent_executions = 2`, env vars for secret + hostname + zone ID.
4. `deploy/tofu/apigw.tf`: HTTP API with `aws_apigatewayv2_route` for `GET /nic/update` + `POST /nic/update`, throttle_burst_limit = 100, throttle_rate_limit = 10.
5. `deploy/tofu/iam.tf`: role with `route53:ChangeResourceRecordSets` scoped via condition `route53:ChangeResourceRecordSetsRecordTypes = ["A"]` and `route53:ChangeResourceRecordSetsNormalizedRecordNames = [<hostname>]` — pattern lifted from `docs/aws-setup.md`.
6. `deploy/tofu/outputs.tf`: `api_url`, `curl_test_command`, `unifi_ui_server_field`.
7. `deploy/tofu/README.md`: three-step deploy (init, apply, paste output into UniFi UI).

Acceptance: `tofu init && tofu plan` validates with no errors against a real AWS account; `tofu apply` produces a working endpoint.

### Phase 3 — Docs + Makefile integration

1. `Makefile`: `build-lambda` target that produces `dist/lambda.zip`.
2. `docs/unifi-inadyn-bridge.md`:
   - Who this is for (UniFi users wanting event-driven, willing to add one Lambda to their AWS account)
   - Architecture sketch (copy from this plan)
   - Cost table (copy from this plan)
   - Deploy steps: `make build-lambda` + `cd deploy/tofu && tofu apply -var=hosted_zone_id=Z... -var=hostname=... -var=shared_secret=...`
   - UniFi UI config: Service `Custom`, Hostname, Username `dddns`, Password `<shared_secret>`, Server `<api_url>/nic/update?hostname=%h&myip=%i`
   - Rotation: `tofu apply -var=shared_secret=<new>` + update UniFi UI
   - Teardown: `tofu destroy`
3. `docs/README.md` + `docs/udm-guide.md`: add "Alternative: event-driven via AWS Lambda bridge" paragraph linking to `unifi-inadyn-bridge.md`.
4. `ai_docs/5_unifi-ddns-bridge.md`: the original design doc — mark "serve mode on UniFi was deemed incompatible; Lambda bridge is the event-driven answer for UDM/UDR. See docs/unifi-inadyn-bridge.md."

### Phase 4 — End-to-end validation on UDR7

1. Deploy the Lambda (`make build-lambda && cd deploy/tofu && tofu apply ...`).
2. Configure UniFi UI with the `api_url` output.
3. Trigger a push (toggle the UI entry off/on).
4. Verify CloudWatch log shows the request.
5. Verify Route53 A record updated.
6. `tofu destroy` afterward — prove the stack is fully idempotent.

## Operational concerns

- **Secret rotation:** bump `shared_secret` variable, `tofu apply`, paste new value into UniFi UI. Takes ~30s. No Lambda code change.
- **IAM credential in OpenTofu state file:** the shared secret ends up in `terraform.tfstate` as plaintext. Two mitigations: (a) `.tfstate` in a remote backend (S3 + DynamoDB lock + encryption at rest) — standard OpenTofu practice; (b) use SSM Parameter Store SecureString instead of env var, referenced from Lambda at cold start. For a single-user home deployment, (a) is overkill; local tfstate with `chmod 600` is acceptable. Document the trade-off.
- **CloudWatch costs:** 7-day retention + <1 KB per request means even 10k requests/year = negligible ingest. No need to tune beyond the default.
- **Lambda cold starts:** Go on provided.al2023 + 128 MB ≈ 200-400ms cold start. inadyn doesn't care about latency at this scale; users will.

## Non-goals

- **"Make dddns talk to Lambda instead of Route53."** Lambda is a receiver for UniFi's push, not a middleman for dddns's own updates. dddns on cron mode continues to call Route53 directly with its SigV4 client.
- **"Support other clients besides inadyn."** The Lambda accepts dyndns v2 semantics, which inadyn speaks. Other clients speaking the same protocol work by accident. We don't claim broad compatibility.
- **"CDN / custom domain."** API Gateway's native `execute-api` URL is fine — not user-facing, not indexed by search engines. Adding CloudFront raises the cost floor for zero real benefit in this use case.
- **"Lockout state via DynamoDB."** API Gateway throttling + Lambda concurrency cap are the bound on abuse. Adding DynamoDB doubles the operational surface and adds a cost line for marginal gain.

## Acceptance criteria

This plan is complete when:

1. `lambda/` directory contains handler code with ≥90% test coverage via `go test ./lambda/...`.
2. `deploy/tofu/` is `tofu plan`-clean and `tofu apply`-succeeds against a fresh AWS account.
3. `make build-lambda` produces a zip file suitable for Lambda upload.
4. `docs/unifi-inadyn-bridge.md` walks a user from "I have an AWS account" to "my UniFi DDNS is event-driven" in under 20 minutes.
5. End-to-end test on UDR7 validates the path UniFi UI → inadyn → API Gateway → Lambda → Route53 UPSERT, with the CloudWatch log showing the request.
6. `tofu destroy` leaves a clean AWS account (no orphaned resources).

## Estimated size

- **Go code:** ~200 LoC for handler + ~100 LoC for tests.
- **OpenTofu:** ~150 LoC across 6 files.
- **Docs:** ~200 lines.
- **Makefile addition:** ~10 lines.

Total: one afternoon's work if the IAM scoping doesn't fight us. Two if it does.

## Relationship to existing dddns features

| Existing feature | Lambda bridge interaction |
|---|---|
| Cron mode (polling) | Unchanged. Still the recommended UniFi path for users who don't want AWS Lambda in their stack. |
| Serve mode (loopback) | Unchanged. Still the recommended path for same-host DDNS clients (Pi, Linux server, Docker sidecar). |
| `internal/dns/` SigV4 + Route53 REST client | **Reused verbatim** by the Lambda. Zero duplication. |
| `internal/config/` | Not used by Lambda — Lambda reads env/SSM, not config files. |
| `internal/server/` serve-mode handler | Separate code path. Serve handler bound to loopback, Lambda handler bound to API Gateway invocation context. |
| Installer (`scripts/install-on-unifi-os.sh`) | Unchanged. Lambda has its own deploy path via `tofu apply`. |

## Why this lives in the dddns repo

- **Shared Route53 client:** the Lambda handler's `UpdateIP` call goes through the same Go code dddns cron mode uses. Forking into a sibling repo would duplicate SigV4 + REST + error parsing — the exact stuff we just hand-rolled and tested. Keeping it in-repo means one signer, one test suite, one version to maintain.
- **Release cadence:** Lambda handler changes ship alongside dddns binary changes. A SigV4 bug fix benefits both.
- **Docs locality:** users evaluating UniFi event-driven options see cron / serve / Lambda all in one place.
- **Scope fit:** the repo is about "keep home network DNS current on AWS Route53". The Lambda is one of four mechanisms for that (the others being cron/serve/future-providers). It belongs here.

Net negative: the repo grows from pure Go + shell → Go + shell + HCL. Acceptable one-time cost for the maintenance win.
