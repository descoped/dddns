# SecureString parameter for the dyndns shared secret.
#
# Bootstrap flow:
#   1. tofu apply creates the parameter with a random placeholder.
#   2. Operator runs ../scripts/rotate-secret.sh, which calls
#      `aws ssm put-parameter --overwrite` and prints the new
#      secret in a framed block for pasting into UniFi UI.
#   3. ignore_changes = [value] means subsequent `tofu apply` runs
#      don't clobber the rotated value.
#
# The random placeholder is never used in production — the rotate
# script replaces it before any real client pushes. Its sole purpose
# is to get a valid parameter into place so the Lambda's first cold
# start doesn't crash on an empty SSM fetch.

resource "random_password" "bootstrap_secret" {
  length  = 32
  special = false
}

resource "aws_ssm_parameter" "shared_secret" {
  name        = var.ssm_parameter_name
  description = "dddns dyndns-v2 shared secret (rotated out-of-band; see deploy/aws-lambda/scripts/rotate-secret.sh)"
  type        = "SecureString"
  value       = random_password.bootstrap_secret.result
  tier        = "Standard"

  tags = local.common_tags

  lifecycle {
    ignore_changes = [value]
  }
}
