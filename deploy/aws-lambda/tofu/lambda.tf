# Lambda function + its CloudWatch log group.

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.name}"
  retention_in_days = var.log_retention_days
  tags              = local.common_tags
}

resource "aws_lambda_function" "dddns" {
  function_name = local.name
  role          = aws_iam_role.lambda.arn

  # provided.al2023 + arm64 gives us the cheapest and fastest-cold-start
  # combination available on Lambda. The 'bootstrap' filename inside the
  # zip is required by the provided.* runtime family.
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  handler       = "bootstrap"

  filename         = var.lambda_zip_path
  source_code_hash = filebase64sha256(var.lambda_zip_path)

  memory_size                    = var.lambda_memory_mb
  timeout                        = var.lambda_timeout_seconds
  reserved_concurrent_executions = var.reserved_concurrency

  environment {
    variables = {
      # All values the Lambda reads at init — see deploy/aws-lambda/main.go.
      HOSTED_ZONE_ID   = var.hosted_zone_id
      DDDNS_HOSTNAME   = var.hostname
      SSM_SECRET_PARAM = var.ssm_parameter_name
      # GOMEMLIMIT caps the Go soft heap. The handler allocates <1 MB per
      # request; 16 MiB gives ample headroom while staying far below the
      # 128 MB Lambda memory allocation (leaving room for runtime overhead).
      GOMEMLIMIT = "16MiB"
    }
  }

  depends_on = [
    aws_cloudwatch_log_group.lambda,
    aws_iam_role_policy.route53,
    aws_iam_role_policy.ssm,
  ]

  tags = local.common_tags
}
