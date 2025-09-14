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

### Step 3: Create Custom Policy

1. Click **Create policy** → **JSON** tab
2. Replace the content with this minimal policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowRoute53Updates",
      "Effect": "Allow",
      "Action": [
        "route53:ChangeResourceRecordSets",
        "route53:GetHostedZone",
        "route53:ListResourceRecordSets"
      ],
      "Resource": "arn:aws:route53:::hostedzone/Z1234567890ABC"
    },
    {
      "Sid": "AllowGetChange",
      "Effect": "Allow",
      "Action": [
        "route53:GetChange"
      ],
      "Resource": "arn:aws:route53:::change/*"
    }
  ]
}
```

3. **IMPORTANT**: Replace `Z1234567890ABC` with your actual Hosted Zone ID
4. Click **Next: Tags** → **Next: Review**
5. Name: `dddns-route53-access`
6. Description: `Minimal permissions for dddns to update DNS records`
7. Click **Create policy**

### Step 4: Attach Policy and Create User

1. Back in Create User flow, click **Refresh**
2. Search for and select `dddns-route53-access`
3. Click **Next: Tags** → **Next: Review** → **Create user**
4. **CRITICAL**: Save the credentials:
   - Access Key ID: `AKIAXXXXXXXXXXXXXX`
   - Secret Access Key: `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

> **⚠️ Warning**: This is the ONLY time you can see the Secret Access Key. Save it securely!

### Step 5: Configure AWS CLI Profile

On your device where dddns will run:

```bash
aws configure --profile dddns
```

Enter when prompted:
```
AWS Access Key ID: AKIAXXXXXXXXXXXXXX
AWS Secret Access Key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Default region name: us-east-1
Default output format: json
```

This creates `~/.aws/credentials` with your secure credentials.

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
- Only grant permissions for specific hosted zone
- Don't use `Resource: "*"` in policies

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
**Solution**: Check IAM policy has correct Hosted Zone ID

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

1. Return to [Quick Start Guide](QUICK_START.md) to install dddns
2. Use your Hosted Zone ID in dddns configuration
3. Configure AWS profile with IAM user credentials
4. Test with `dddns update --dry-run`

## Additional Resources

- [Route53 Documentation](https://docs.aws.amazon.com/route53/)
- [IAM Best Practices](https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html)
- [Route53 Pricing](https://aws.amazon.com/route53/pricing/)
- [DNS Basics](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/dns-basics.html)