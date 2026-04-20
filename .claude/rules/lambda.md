# AWS Lambda Deployment

Applies to: `deploy/aws-lambda/**`.

## Package layout

- `main.go` — Lambda entry + init-time AWS client construction
- `handler.go` — dyndns v2 handler; ignores `myip`, publishes `requestContext.http.sourceIp`
- `ssm.go` — hand-rolled SSM GetParameter (reuses `internal/dns` SigV4 signer — no `aws-sdk-go-v2` dependency)
- `handler_test.go` — httptest-backed unit tests (auth / hostname / sourceIp / SSM cache)
- `tofu/` — OpenTofu module, 13 AWS resources, all operator inputs as variables
- `scripts/rotate-secret.sh` — `openssl rand` + `aws ssm put-parameter --overwrite`
- `README.md` — operator-facing deploy guide (authoritative; this file is the short form)

## Deployment flow

```bash
just build-aws-lambda                                  # Linux arm64 zip for provided.al2023
cd deploy/aws-lambda/tofu && tofu init && tofu apply   # 13 AWS resources
cd .. && ./scripts/rotate-secret.sh                    # rotate shared secret; print for paste
# Paste the printed secret into UniFi UI → Dynamic DNS → Password
```

## Secret rotation (Lambda mode)

Rotation principle: **tofu owns the storage slot, operator owns the value.** Tofu creates the SSM parameter with a throwaway bootstrap placeholder and `lifecycle { ignore_changes = [value] }`, then steps aside. No subsequent `tofu apply` touches the secret.

Rotation script: `deploy/aws-lambda/scripts/rotate-secret.sh`

1. **Generate** — `openssl rand -hex 32` → 256-bit secret in shell memory (never written to disk or log).
2. **Store** — `aws ssm put-parameter --overwrite` replaces the SecureString value. SSM re-encrypts server-side with `alias/aws/ssm`; parameter version bumps.
3. **Print** — framed block to stdout, exactly once, for pasting into the DDNS client's password field. Shell exits → plaintext gone.
4. **Propagate** — Lambda picks up the new value within ≤60 s (in-process cache in `deploy/aws-lambda/ssm.go`). During the overlap window, both old and new secrets authenticate — no gap in push traffic.

Run from `deploy/aws-lambda/`. The script auto-reads the parameter name and region from `tofu output`; override with `-n /path -r region` for standalone use.

## Why tofu does not perform rotation

Rotation must display the new value to the operator *once*, for UI paste. Tofu has no output channel that reveals a sensitive value without also committing it to state. The shell script is the correct seam: one-shot, ephemeral, no persistence beyond the SSM put.

## IAM scoping (invariant — do not widen)

Lambda's execution role grants:

- **Route53 `ChangeResourceRecordSets`** — condition keys pin it to one zone, one record name, `UPSERT` only, record type `A`. No `DeleteResourceRecordSets`.
- **SSM `GetParameter`** — one parameter ARN.
- **KMS `Decrypt`** — gated on `kms:ViaService = ssm.<region>.amazonaws.com`.
- **CloudWatch Logs write** — via `AWSLambdaBasicExecutionRole` managed policy.

Do not add `DeleteResourceRecordSets`, multi-record access, or multi-parameter SSM access. This is arguably the tightest policy possible for the operation.

## Dependency posture

- The Lambda binary reuses `internal/dns/sigv4.go` for both Route53 and SSM signing. **Do not pull in `aws-sdk-go-v2/service/ssm`** — the point is a <10 MB binary and fast cold start.
- `aws_lambda_permission` and `random_password` don't support AWS tagging; every other resource carries the full tag set.

## Common gotchas

- **401 on the first push after `tofu apply`** — parameter still holds the bootstrap placeholder. Run the rotate script.
- **Old secret still works for ≤60 s after rotation** — normal, in-process cache. Not a bug.
- **Missed the copy?** Cannot retrieve from SSM (encrypted at rest; the rotate script is the only moment the secret exists in plaintext). Safe to re-run — `--overwrite` is idempotent.
- **Reusability** — zero hardcoded account IDs, zone IDs, or hostnames in any `.tf` file; resource names derive from `${var.name_prefix}-${replace(var.hostname, ".", "-")}`. Two deployments in the same account do not collide.
