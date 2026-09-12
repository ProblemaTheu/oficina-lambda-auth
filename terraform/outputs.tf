output "gateway_url" {
  description = "URL pública da API"
  # O provider marca todo valor de SSM como sensível. A URL pública da API
  # não é segredo — é o que vai no README e no vídeo.
  value = nonsensitive(data.aws_ssm_parameter.apigw_url.value)
}

output "funcoes" {
  description = "Nomes das funções Lambda"
  value       = { for k, f in aws_lambda_function.fn : k => f.function_name }
}

output "authorizer_id" {
  description = "ID do authorizer no API Gateway"
  value       = aws_apigatewayv2_authorizer.jwt.id
}
