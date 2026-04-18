# dddns — AWS Lambda deployment form

An alternative to cron and serve mode: run dddns as a small AWS
Lambda function behind an API Gateway HTTPS endpoint. A DDNS push
client (typically UniFi Dream's built-in `inadyn`) pushes updates
to the endpoint, which triggers a Route53 UPSERT.

## When to use this

| Deployment | Fits when | Drawback |
|------------|-----------|----------|
| **cron** | You can run a scheduled binary on the same host as the WAN interface (UniFi, Raspberry Pi, Linux server). | 30-min polling delay on IP change. |
| **serve** | You have a DDNS client running on the same host as the listener (ddclient, a user script, a Docker sidecar). Event-driven. | UniFi Dream's built-in `inadyn` cannot reach the loopback listener due to its `-b eth4` binding — see `docs/udm-guide.md`. |
| **lambda** (this) | UniFi UI's Custom Dynamic DNS is the push source and you want event-driven updates without running anything on the router. Also a good fit if the LAN/router is unreliable and a cloud endpoint is more stable. | Costs a few cents per month. Requires an AWS account + `tofu`. |

Lambda costs scale with push frequency. A household-scale deployment
(a handful of pushes per day) stays firmly in AWS's free tier — Lambda,
API Gateway HTTP API, SSM Parameter Store Standard tier, and CloudWatch
Logs (7-day retention) are all free at this volume.

## What this deploys

```
API Gateway HTTP API (throttle 10 rps / burst 100)
         │
         ▼
Lambda (provided.al2023, arm64, 128 MB, 10 s timeout, concurrency 2)
   ├── reads shared-secret from SSM SecureString
   ├── constant-time-compares Basic Auth header
   └── Route53 UPSERT A record (scoped IAM, UPSERT only)
         │
         ▼
CloudWatch Logs (7-day retention)
```

The Lambda ignores the `myip=` query parameter entirely; the only IP
it will ever publish is the TCP source address recorded by API Gateway
(`requestContext.http.sourceIp`). This mirrors the "never trust
client-supplied values" posture the serve mode enforces with
`wanip.FromInterface`.

## Prerequisites

- **OpenTofu** 1.6+ (or Terraform 1.5+). `brew install opentofu` on macOS.
- **AWS CLI v2**, authenticated against the target account with permission to create IAM roles, Lambda functions, API Gateway HTTP APIs, SSM parameters, and CloudWatch log groups. `aws sts get-caller-identity` should return your user/role.
- **Go 1.26+** to build the Lambda binary (only needed on the deploy host; end users never compile).
- **Route53 hosted zone** already containing the A record you want to update. The IAM policy scopes the Lambda to UPSERT-only on exactly that one record.

## Configure your deployment

Copy `tofu/terraform.tfvars.example` → `tofu/terraform.tfvars` and
fill in your values. The file is gitignored (both `terraform.tfvars`
and `*.local.tfvars` under `deploy/**/`), so your real zone ID,
hostname, and any other deployment-specific values never reach git.

```bash
cd deploy/aws-lambda/tofu
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars      # set hosted_zone_id and hostname at minimum
```

Only two values are strictly required: `hosted_zone_id` and
`hostname`. Everything else has a default — see `variables.tf` for
the full list or `terraform.tfvars.example` for the commented menu.

### AWS credentials

The AWS provider reads credentials from the standard AWS CLI
locations — you don't need to configure anything in this module.
Whichever account your `aws sts get-caller-identity` resolves to is
the one `tofu apply` will deploy into.

If you use named profiles (`aws configure --profile descoped`),
select one at apply time:

```bash
# Named profile
AWS_PROFILE=descoped tofu apply

# Or export for the whole shell session
export AWS_PROFILE=descoped
tofu apply
```

If you have only a default profile, no extra step needed — just
`tofu apply`.

## Three-step deploy

From the repository root, assuming you've configured `terraform.tfvars`
as above:

```bash
# 1. Build the Lambda zip (produces deploy/aws-lambda/dist/lambda.zip)
just build-aws-lambda

# 2. Apply the OpenTofu module. tofu auto-loads terraform.tfvars.
cd deploy/aws-lambda/tofu
tofu init
AWS_PROFILE=descoped tofu apply   # or just 'tofu apply' if using default profile

# 3. Rotate the shared secret. The tofu apply creates the SSM
#    parameter with a random placeholder; the helper replaces it
#    with a fresh 256-bit value and prints it for paste.
cd ..
AWS_PROFILE=descoped ./scripts/rotate-secret.sh
```

The rotate script prints the new secret in a framed block. Copy it
into UniFi UI → Internet → Dynamic DNS:

| Field | Value |
|-------|-------|
| **Service** | Custom |
| **Hostname** | (match your `hostname` variable — e.g. `home.example.com`) |
| **Username** | anything non-empty (the Lambda ignores the username and auths on the secret only) |
| **Password** | (the value from rotate-secret.sh) |
| **Server** | (copy from `tofu output unifi_ui_server_field`) |

The `Server` output looks like:
```
<api-id>.execute-api.<region>.amazonaws.com/nic/update?hostname=%h&myip=%i
```

UniFi's inadyn sends `myip=%i` because the protocol requires it, but
the Lambda throws it away — the IP published is the API Gateway
source IP. This prevents a compromised client from publishing
arbitrary IPs.

## Smoke test

```bash
# From the tofu output
tofu output curl_test_command
# Will print:
# curl -u 'dddns:YOUR_SECRET' 'https://…/nic/update?hostname=home.example.com&myip=198.51.100.1'

# Paste the command, replace YOUR_SECRET with the rotate-secret output,
# and run. Expected response:
#   good <your-actual-public-ip>
# The myip=198.51.100.1 is ignored — your real sourceIp is what
# shows up in the DNS record and in the response body.
```

Watch the Lambda run in real time:
```bash
aws logs tail $(tofu output -raw cloudwatch_log_group) --follow
```

Check the Route53 record actually moved:
```bash
dig +short home.example.com @1.1.1.1
```

## Rotating the secret

Any time — the operation is non-disruptive:

```bash
cd deploy/aws-lambda
./scripts/rotate-secret.sh
```

Paste the printed value into UniFi UI. The Lambda caches the secret
for 60 seconds between invocations (keeps SSM cost negligible), so:

- During the 60-second overlap window **both** the old and new
  secrets authenticate successfully.
- After the window, only the new secret works.
- UniFi's inadyn cache also bridges the window, so there's no
  practical race.

## Costs

At UniFi's default DDNS push cadence (a few per day unless the WAN IP
changes), everything sits comfortably in the AWS free tier:

- **Lambda**: 1M free requests per month + 400 k GB-seconds compute.
- **API Gateway HTTP API**: 1M free requests per month (first 12 months of account), $1.00/M afterwards.
- **SSM Parameter Store** (Standard tier): free for up to 10 k parameters + 10k API calls per month.
- **CloudWatch Logs**: 5 GB ingestion free per month; 7-day retention keeps storage well under this.

Practical estimate for a personal deployment: **$0/month for the first year, ~$0.01–$0.05/month afterwards.**

## Teardown

Everything the module created is owned by the module — `tofu destroy`
cleanly removes it all (Lambda, API, SSM param, IAM role, log group):

```bash
cd deploy/aws-lambda/tofu
AWS_PROFILE=descoped tofu destroy   # picks up terraform.tfvars automatically
```

This does **not** touch your Route53 records — the A record you
pointed dddns at stays in the zone.

## Cost attribution

Every resource the module creates is tagged with:

| Key | Value |
|---|---|
| `app` | `dddns` |
| `hostname` | value of `var.hostname` |
| `module` | `deploy/aws-lambda` |

Plus anything you add via `var.tags`.

**The Route53 hosted zone itself is not managed by this module** (it
pre-dates the deployment and is user-owned — letting `tofu destroy`
ever reach it would be a footgun). Tag it manually for complete cost
attribution:

```sh
aws route53 change-tags-for-resource \
  --resource-type hostedzone \
  --resource-id <YOUR_ZONE_ID> \
  --add-tags Key=app,Value=dddns Key=module,Value=route53-zone Key=hostname,Value=<YOUR_HOSTNAME>
```

Zone tags persist across `tofu apply` / `tofu destroy` — no drift.

**Enable Cost Explorer filtering** (one-time, **management account
only** in an AWS Organization — member accounts get
`AccessDeniedException`):

```sh
aws ce update-cost-allocation-tags-status \
  --cost-allocation-tags-status \
    TagKey=app,Status=Active \
    TagKey=hostname,Status=Active \
    TagKey=module,Status=Active
```

Activation takes up to 24 h for historical spend to backfill.
Once active, Cost Explorer's **Tag → app = dddns** filter rolls up
Lambda + API Gateway + CloudWatch + SSM + Route53 hosting & queries.

## All deployment variables

All defined in `tofu/variables.tf`. Required: `hosted_zone_id`,
`hostname`. Everything else has a sensible default.

| Variable | Default | Notes |
|----------|---------|-------|
| `hosted_zone_id` | — | Route53 zone ID (`Z…`). Required. |
| `hostname` | — | FQDN of the A record. Required. |
| `aws_region` | `us-east-1` | Pick a region close to you for lower latency. Route53 itself is global. |
| `name_prefix` | `dddns` | Prefix for every created resource. |
| `ssm_parameter_name` | `/dddns/shared_secret` | SSM path for the shared secret. |
| `reserved_concurrency` | `2` | Ceiling on concurrent Lambda executions. |
| `log_retention_days` | `7` | CloudWatch Logs retention. |
| `lambda_memory_mb` | `128` | More memory = more CPU. 128 is plenty. |
| `lambda_timeout_seconds` | `10` | Per-invocation budget. |
| `throttle_burst` | `100` | API Gateway burst ceiling. |
| `throttle_rate` | `10` | API Gateway sustained rate ceiling (per second). |
| `lambda_zip_path` | `../dist/lambda.zip` | Output of `just build-aws-lambda`. |
| `tags` | `{}` | Merged into the built-in `app=dddns` / `hostname=<...>` tags on every resource. |

For a personal-DDNS deployment you typically only need to pass the
two required variables; everything else is fine on defaults.

## Troubleshooting

**`tofu apply` fails with `AccessDenied` creating IAM role**

Your AWS CLI user/role needs `iam:CreateRole`, `iam:PutRolePolicy`,
and friends. A developer/administrator role is the common path; the
least-privilege set is the union of all `aws_iam_*` and `aws_lambda_*`
and `aws_apigatewayv2_*` and `aws_ssm_parameter` resources in the
module's `tofu plan`.

**UniFi UI shows `badauth` repeatedly**

Run `./scripts/rotate-secret.sh` and paste the new value into UniFi
UI. The old secret may have drifted out of sync, or the SSM parameter
was never rotated from the random bootstrap placeholder.

**`dnserr` responses — Route53 IAM**

```bash
aws logs tail $(cd tofu && tofu output -raw cloudwatch_log_group) --follow
```

The Lambda logs the Route53 error verbatim. Most common cause: the
record name in the condition block doesn't exactly match what
Route53 normalises the hostname to (lowercased, no trailing dot).

**Changing the hostname**

The IAM policy is scoped per hostname at tofu-apply time. Changing
`var.hostname` and re-running `tofu apply` will update the policy
and the Lambda env in place — no data migration needed. The old
A record (if it's still in the zone) continues to resolve normally;
dddns just stops updating it.

**Logs say "ssm fetch failed"**

The Lambda couldn't reach SSM or couldn't decrypt the parameter.
Check the execution role — the module grants `ssm:GetParameter`
on exactly the one parameter ARN plus `kms:Decrypt` on the
`alias/aws/ssm` key. If you moved the parameter to a customer-
managed KMS key, you'll need to adjust `iam.tf` accordingly.
