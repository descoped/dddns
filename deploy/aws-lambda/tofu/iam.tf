# Lambda execution role — scoped tightly to the two operations the
# handler actually performs:
#
#   1. route53:ChangeResourceRecordSets on exactly one zone + one
#      record name + action=UPSERT.
#   2. ssm:GetParameter on exactly one parameter ARN + the KMS key
#      that decrypts it.
#
# No '*' resource wildcards anywhere. This is the same scoping model
# as docs/aws-setup.md's recommended IAM policy for the cron path.

data "aws_iam_policy_document" "assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "${local.name}-exec"
  assume_role_policy = data.aws_iam_policy_document.assume_role.json
  tags               = local.common_tags
}

# CloudWatch Logs permissions (managed policy).
resource "aws_iam_role_policy_attachment" "logs" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Route53 — scoped to the one hosted zone + record name, UPSERT only.
data "aws_iam_policy_document" "route53" {
  statement {
    sid    = "UpsertOneRecord"
    effect = "Allow"
    actions = [
      "route53:ChangeResourceRecordSets",
    ]
    resources = [
      "arn:aws:route53:::hostedzone/${var.hosted_zone_id}",
    ]
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "route53:ChangeResourceRecordSetsNormalizedRecordNames"
      values   = [lower(var.hostname)]
    }
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "route53:ChangeResourceRecordSetsActions"
      values   = ["UPSERT"]
    }
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "route53:ChangeResourceRecordSetsRecordTypes"
      values   = ["A"]
    }
  }

  statement {
    sid       = "ReadZoneMetadata"
    effect    = "Allow"
    actions   = ["route53:GetHostedZone", "route53:ListResourceRecordSets"]
    resources = ["arn:aws:route53:::hostedzone/${var.hosted_zone_id}"]
  }
}

resource "aws_iam_role_policy" "route53" {
  name   = "route53-upsert-scoped"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.route53.json
}

# SSM — GetParameter on exactly one parameter.
data "aws_iam_policy_document" "ssm" {
  statement {
    sid    = "ReadSharedSecret"
    effect = "Allow"
    actions = [
      "ssm:GetParameter",
    ]
    resources = [
      aws_ssm_parameter.shared_secret.arn,
    ]
  }

  statement {
    sid    = "DecryptSharedSecret"
    effect = "Allow"
    actions = [
      "kms:Decrypt",
    ]
    # Default AWS-managed SSM KMS key — Parameter Store uses this for
    # SecureString entries unless a customer-managed key is configured.
    resources = [
      "arn:aws:kms:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:alias/aws/ssm",
    ]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${data.aws_region.current.name}.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "ssm" {
  name   = "ssm-get-secret-scoped"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.ssm.json
}
