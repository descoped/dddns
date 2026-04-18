# Data sources + locals shared across the module.

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  # Resource name prefix, normalized lowercase and with dots from
  # hostname turned into dashes so it's a valid AWS resource name.
  name = "${var.name_prefix}-${replace(lower(var.hostname), ".", "-")}"

  common_tags = merge(
    {
      app      = "dddns"
      hostname = var.hostname
      module   = "deploy/aws-lambda"
    },
    var.tags,
  )
}
