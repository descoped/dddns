# AWS Route53 Setup Guide

This guide walks you through setting up AWS Route53 for use with dddns. If you already have a Route53 hosted zone configured, you can skip to [Creating IAM User](#creating-iam-user-for-dddns).

## Table of Contents
- [Creating a Hosted Zone](#creating-a-hosted-zone)
- [Creating IAM User for dddns](#creating-iam-user-for-dddns)
- [Cost Information](#cost-information)
- [Quick Verification](#quick-verification)
- [Security Best Practices](#security-best-practices)
- [Troubleshooting](#troubleshooting)

## Creating a Hosted Zone

A hosted zone is a container for DNS records in Route53. You need one for your domain.

### Step 1: Sign in to AWS Console
- Navigate to [Route53 Console](https://console.aws.amazon.com/route53/)
- If new to Route53, choose **Get started** under DNS management
- Otherwise, choose **Hosted zones** in the navigation pane

### Step 2: Create the Hosted Zone
```
Click: Create hosted zone
Domain name: example.com (your actual domain)
Type: Public hosted zone
Tags: (optional)
Click: Create hosted zone
```

### Step 3: Note the Name Servers
After creation, you'll see 4 name servers:
```
ns-1234.awsdns-12.org
ns-5678.awsdns-34.co.uk
ns-9012.awsdns-56.com
ns-3456.awsdns-78.net
```
Copy these - you'll need them for the next step.

### Step 4: Update Your Domain Registrar

At your domain registrar (GoDaddy, Namecheap, Google Domains, etc.):
1. Log into your registrar's control panel
2. Find DNS or Name Server settings
3. Replace existing name servers with the Route53 ones
4. Save changes

> **⚠️ Important**: DNS propagation can take up to 48 hours, but typically completes within 2-4 hours.

### Step 5: Copy the Hosted Zone ID

1. In Route53 console, click on your domain
2. Copy the **Hosted Zone ID** (format: `Z1234567890ABC`)
3. Save this - you'll need it for dddns configuration

## Creating IAM User for dddns

For security, create a dedicated IAM user with minimal permissions.

### Step 1: Open IAM Console
- Navigate to [IAM Console](https://console.aws.amazon.com/iam/)
- Click **Users** → **Create user**

### Step 2: Configure User
```
User name: dddns-updater
Access type: ✓ Programmatic access
Click: Next: Permissions
```

### Step 3: Create the Scoped Policy

The policy below is the one dddns is designed to run under. It is deliberately narrower than the AWS-managed `AmazonRoute53FullAccess` or a simple zone-wide permission — it uses Route53 condition keys to restrict the IAM user to a **single record name, single record type, single action** (`UPSERT`). Nothing else is reachable with these credentials.

1. Click **Create policy** → **JSON** tab
2. Paste the following, then replace the two placeholders:
   - `ZXXXXXXXXXXXXX` → your Hosted Zone ID
   - `home.example.com` → the exact A record name dddns will manage

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ListZoneForLookup",
      "Effect": "Allow",
      "Action": "route53:ListResourceRecordSets",
      "Resource": "arn:aws:route53:::hostedzone/ZXXXXXXXXXXXXX"
    },
    {
      "Sid": "UpsertSingleARecord",
      "Effect": "Allow",
      "Action": "route53:ChangeResourceRecordSets",
      "Resource": "arn:aws:route53:::hostedzone/ZXXXXXXXXXXXXX",
      "Condition": {
        "ForAllValues:StringEquals": {
          "route53:ChangeResourceRecordSetsNormalizedRecordNames": ["home.example.com"],
          "route53:ChangeResourceRecordSetsRecordTypes": ["A"],
          "route53:ChangeResourceRecordSetsActions": ["UPSERT"]
        }
      }
    }
  ]
}
```

3. Click **Next: Tags** → **Next: Review**
4. Name: `dddns-route53-upsert`
5. Description: `UPSERT single A record for dddns — condition-key scoped`
6. Click **Create policy**

#### Why this shape?

With this policy the IAM credentials **cannot**:

- delete any record (no `DELETE` action)
- create or modify any other record (name or type is rejected by the condition)
- change the TTL on a record the policy doesn't already allow
- touch `NS`, `MX`, `TXT`, `CNAME`, `AAAA`, or `SOA` records anywhere in the zone
- reach any other hosted zone (the `Resource` is one ARN)

Combined with dddns's own "local WAN IP is authoritative" defense (the serve-mode handler ignores the client-supplied `myip` and reads the real interface), the practical blast radius of a full credential leak is: **an attacker can cause an A record to point to the router's real current IP, which is where it already points**. That's the design target.

#### Multi-record setups

If you run dddns for two hostnames (say `home.example.com` and `router.example.com`), list both in the condition:

```json
"route53:ChangeResourceRecordSetsNormalizedRecordNames": [
  "home.example.com",
  "router.example.com"
]
```

If you actually want broader permissions for some operational reason, the less-scoped form is the zone-wide policy in older guides:

```json
{
  "Effect": "Allow",
  "Action": "route53:ChangeResourceRecordSets",
  "Resource": "arn:aws:route53:::hostedzone/ZXXXXXXXXXXXXX"
}
```

It works but loses the blast-radius guarantees above — if the credentials leak, the attacker owns every record in the zone. Only go this way with eyes open.

### Step 4: Attach Policy and Create User

1. Back in Create User flow, click **Refresh**
2. Search for and select `dddns-route53-access`
3. Click **Next: Tags** → **Next: Review** → **Create user**
4. **CRITICAL**: Save the credentials:
   - Access Key ID: `AKIAXXXXXXXXXXXXXX`
   - Secret Access Key: `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

> **⚠️ Warning**: This is the ONLY time you can see the Secret Access Key. Save it securely!

### Step 5: Put the credentials into dddns config

dddns reads AWS credentials **directly from its own config file** — not from `~/.aws/credentials`, environment variables, or named AWS CLI profiles. Run the interactive wizard or edit the config directly:

```bash
dddns config init    # interactive
```

Or edit `~/.dddns/config.yaml` (UDM / UDR: `/data/.dddns/config.yaml`):

```yaml
aws_region: "us-east-1"
aws_access_key: "AKIAXXXXXXXXXXXXXX"
aws_secret_key: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
hosted_zone_id: "Z1234567890ABC"
hostname: "home.example.com"
ttl: 300
```

Fix permissions if needed — dddns refuses to load `config.yaml` at anything looser than `0600`:

```bash
chmod 600 ~/.dddns/config.yaml
```

For encrypted-at-rest storage (device-specific AES-256-GCM key), run `dddns secure enable` after populating the plaintext config.

## Cost Information

AWS Route53 pricing (as of 2024):

| Service | Cost | Notes |
|---------|------|-------|
| Hosted Zone | $0.50/month | Per zone, first 25 zones |
| DNS Queries | $0.40/million | First billion queries/month |
| **Typical Home Use** | **~$0.50/month** | Minimal queries |

For comparison:
- DynDNS: $55/year (~$4.58/month)
- No-IP: $24.95/year (~$2.08/month)
- Route53: $6/year (~$0.50/month)

## Quick Verification

Test your setup before configuring dddns:

### Check Hosted Zone
```bash
# List all hosted zones
aws route53 list-hosted-zones --profile dddns --output table

# Get specific zone details
aws route53 get-hosted-zone --id Z1234567890ABC --profile dddns
```

### Check DNS Propagation
```bash
# Check if Route53 name servers are active
dig NS example.com

# Should return your Route53 name servers
```

### Test IAM Permissions
```bash
# List records in your zone
aws route53 list-resource-record-sets \
  --hosted-zone-id Z1234567890ABC \
  --profile dddns
```

## Security Best Practices

### 1. Use IAM User, Not Root
- **Never** use AWS root account credentials
- Create dedicated IAM users for each service

### 2. Principle of Least Privilege
- Use the scoped policy above — it restricts by record name, type, and action, not just by hosted zone.
- Don't use `Resource: "*"` in policies.
- Don't attach `AmazonRoute53FullAccess` to dddns credentials — the managed policy can modify NS/SOA records and delete entire zones.

### 3. Credential Management
- Rotate access keys every 90 days
- Use AWS profiles (`~/.aws/credentials`)
- Never commit credentials to version control
- Consider using AWS Secrets Manager for production

### 4. Enable MFA
- Add MFA to your AWS root account
- Consider MFA for IAM users with console access

### 5. Monitor Usage
- Enable CloudTrail for audit logging
- Set up billing alerts
- Review IAM access reports regularly

## Troubleshooting

### "AccessDenied" Error
```
Error: AccessDeniedException: User is not authorized to access this resource
```
**Solutions**:
1. Confirm the Hosted Zone ID in the policy matches the one in your config.
2. With the scoped policy, the record name in the condition block must match exactly. `home.example.com` in the policy ≠ `HOME.example.com` in the config ≠ `home.example.com.` (trailing dot). Route53 normalises to lowercase and no trailing dot — align the policy to that shape.
3. Only `UPSERT` is allowed — if dddns ever tries `DELETE` or `CREATE`, the condition rejects it. Deletion is not a supported dddns operation.

### "InvalidChangeBatch" Error
```
Error: InvalidChangeBatch: RRSet with DNS name example.com. is not permitted in zone
```
**Solution**: Ensure hostname in config matches your hosted zone domain

### DNS Not Updating
1. Verify name servers are set at registrar
2. Check DNS propagation: `dig example.com`
3. Confirm hosted zone ID is correct
4. Test with AWS CLI directly

### "Throttling" Error
```
Error: Throttling: Rate exceeded
```
**Solution**: Route53 allows 5 requests/second. dddns respects this limit, but check for other applications using the same credentials.

## Next Steps

1. Return to [Quick Start Guide](quick-start.md) to install dddns
2. Use your Hosted Zone ID in dddns configuration
3. Configure AWS profile with IAM user credentials
4. Test with `dddns update --dry-run`

## Additional Resources

- [Route53 Documentation](https://docs.aws.amazon.com/route53/)
- [IAM Best Practices](https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html)
- [Route53 Pricing](https://aws.amazon.com/route53/pricing/)
- [DNS Basics](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/dns-basics.html)