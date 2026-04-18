# All deployment-specific values live here. Nothing is baked into
# the module — the same module deploys to any AWS account, any
# region, for any Route53 zone + hostname combination.

variable "hosted_zone_id" {
  type        = string
  description = "Route53 hosted zone ID that contains the record to update (e.g. 'Z1ABCDEFGHIJKL')."

  validation {
    condition     = can(regex("^Z[A-Z0-9]+$", var.hosted_zone_id))
    error_message = "hosted_zone_id must look like a Route53 zone ID (starts with 'Z', uppercase alphanumerics)."
  }
}

variable "hostname" {
  type        = string
  description = "FQDN of the A record the Lambda is authorised to update (e.g. 'home.example.com'). Must exist inside the hosted zone — the IAM policy scopes write access to this record name only."

  validation {
    condition     = length(var.hostname) > 0 && !endswith(var.hostname, ".")
    error_message = "hostname must be non-empty and must NOT end in a trailing dot — Route53 normalises automatically."
  }
}

variable "aws_region" {
  type        = string
  description = "AWS region to deploy Lambda, API Gateway, and SSM into. Route53 is global, so this region only affects where the Lambda runs and where its SSM parameter lives."
  default     = "us-east-1"
}

variable "name_prefix" {
  type        = string
  description = "Prefix applied to every created resource name. Useful if you want multiple independent deployments in the same account (e.g. one per hostname)."
  default     = "dddns"
}

variable "ssm_parameter_name" {
  type        = string
  description = "SSM Parameter Store path for the shared secret. Must start with '/'. Rotate the value via scripts/rotate-secret.sh after the first apply."
  default     = "/dddns/shared_secret"

  validation {
    condition     = startswith(var.ssm_parameter_name, "/")
    error_message = "ssm_parameter_name must start with '/'."
  }
}

variable "reserved_concurrency" {
  type        = number
  description = "Reserved concurrent executions for the Lambda. Caps simultaneous invocations — a safety ceiling against cost runaway if a misconfigured client hammers the endpoint. 2 is plenty for a single DDNS push stream."
  default     = 2
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch Logs retention for the Lambda's log group. The default keeps a week of history — long enough to diagnose a mid-week outage, short enough that logs never become a cost item."
  default     = 7
}

variable "lambda_memory_mb" {
  type        = number
  description = "Lambda memory allocation in MB. More memory also grants more CPU; 128 is ample for dddns's single-request workload and keeps pricing at the lowest tier."
  default     = 128
}

variable "lambda_timeout_seconds" {
  type        = number
  description = "Lambda execution timeout in seconds. 10s is comfortable for an SSM GetParameter + Route53 UPSERT round trip (each typically completes in <1s)."
  default     = 10
}

variable "throttle_burst" {
  type        = number
  description = "API Gateway burst rate limit (requests in a short burst before throttling kicks in)."
  default     = 100
}

variable "throttle_rate" {
  type        = number
  description = "API Gateway sustained rate limit (requests per second). Inadyn pushes a few per minute at most; 10/s is well above any legitimate DDNS client."
  default     = 10
}

variable "lambda_zip_path" {
  type        = string
  description = "Path to the pre-built Lambda zip relative to the tofu module. The 'just build-aws-lambda' recipe produces this file."
  default     = "../dist/lambda.zip"
}

variable "tags" {
  type        = map(string)
  description = "Additional tags applied to every AWS resource created by this module. Merged with the built-in 'app=dddns' tag."
  default     = {}
}
