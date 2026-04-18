output "api_url" {
  value       = aws_apigatewayv2_api.dddns.api_endpoint
  description = "Base URL of the API Gateway HTTP API. Append /nic/update for the push endpoint."
}

output "nic_update_endpoint" {
  value       = "${aws_apigatewayv2_api.dddns.api_endpoint}/nic/update"
  description = "The complete URL inadyn should GET against."
}

output "unifi_ui_server_field" {
  value       = "${replace(aws_apigatewayv2_api.dddns.api_endpoint, "https://", "")}/nic/update?hostname=%h&myip=%i"
  description = "Paste into UniFi UI → Internet → Dynamic DNS → Server. The myip=%i is sent by inadyn but ignored by the Lambda (sourceIp wins)."
}

output "ssm_parameter_name" {
  value       = aws_ssm_parameter.shared_secret.name
  description = "SSM parameter holding the shared secret. Rotate via scripts/rotate-secret.sh."
}

output "lambda_function_name" {
  value       = aws_lambda_function.dddns.function_name
  description = "Lambda function name. CloudWatch log group is /aws/lambda/<this name>."
}

output "cloudwatch_log_group" {
  value       = aws_cloudwatch_log_group.lambda.name
  description = "CloudWatch Logs group name. Tail with: aws logs tail <this> --follow"
}

output "curl_test_command" {
  value       = "curl -u 'dddns:YOUR_SECRET' '${aws_apigatewayv2_api.dddns.api_endpoint}/nic/update?hostname=${var.hostname}&myip=198.51.100.1'"
  description = "Smoke-test command. Replace YOUR_SECRET with the value printed by rotate-secret.sh. Expected: 'good <your-real-ip>' — myip is ignored."
}
