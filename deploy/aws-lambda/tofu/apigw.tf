# API Gateway HTTP API — the simpler, cheaper, newer of the two API
# Gateway flavours. Good fit for a single-route Lambda integration.

resource "aws_apigatewayv2_api" "dddns" {
  name          = local.name
  protocol_type = "HTTP"
  description   = "dddns dyndns-v2 receiver (UniFi inadyn → Lambda → Route53)"

  # CORS is off — this endpoint is meant for inadyn-style HTTP clients
  # only; browsers have no legitimate reason to hit /nic/update.
  tags = local.common_tags
}

resource "aws_apigatewayv2_integration" "lambda" {
  api_id                 = aws_apigatewayv2_api.dddns.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.dddns.invoke_arn
  payload_format_version = "2.0"
  integration_method     = "POST"
}

resource "aws_apigatewayv2_route" "nic_update" {
  api_id    = aws_apigatewayv2_api.dddns.id
  route_key = "GET /nic/update"
  target    = "integrations/${aws_apigatewayv2_integration.lambda.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.dddns.id
  name        = "$default"
  auto_deploy = true

  default_route_settings {
    throttling_burst_limit = var.throttle_burst
    throttling_rate_limit  = var.throttle_rate
  }

  tags = local.common_tags
}

# Allow API Gateway to invoke the Lambda.
resource "aws_lambda_permission" "apigw" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.dddns.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.dddns.execution_arn}/*/*"
}
