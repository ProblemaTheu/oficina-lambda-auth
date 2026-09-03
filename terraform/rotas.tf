# Rotas do API Gateway.
#
# A tabela de autorização vem do contrato F3-0.2: o token nasce em
# /auth/token, algumas rotas da aplicação são públicas por natureza, e todo o
# resto de /v1/* passa pelo authorizer antes de tocar o cluster.

locals {
  # Rotas da aplicação que NÃO exigem token, com o caminho no backend.
  # Precisam ser declaradas uma a uma: são mais específicas que o /v1/{proxy+}
  # e é a especificidade que faz o Gateway escolher a rota certa.
  rotas_publicas = {
    login = {
      route_key = "POST /v1/auth/login"
      caminho   = "/v1/auth/login"
    }
    status_os = {
      # Consulta de status por quem não tem conta — o cliente final.
      route_key = "GET /v1/work-orders/{id}/status"
      caminho   = "/v1/work-orders/{id}/status"
    }
    webhook = {
      # Autenticado por assinatura HMAC, não por JWT.
      route_key = "POST /v1/webhooks/budget-response"
      caminho   = "/v1/webhooks/budget-response"
    }
    health = {
      route_key = "GET /health/{proxy+}"
      caminho   = "/health/{proxy}"
    }
  }
}

# ── POST /auth/token — onde o token nasce, portanto pública ─────────────────
resource "aws_apigatewayv2_integration" "token" {
  api_id                 = data.aws_ssm_parameter.apigw_id.value
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.fn["auth-token"].invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "token" {
  api_id    = data.aws_ssm_parameter.apigw_id.value
  route_key = "POST /auth/token"
  target    = "integrations/${aws_apigatewayv2_integration.token.id}"
}

resource "aws_lambda_permission" "token" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.fn["auth-token"].function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${data.aws_apigatewayv2_api.principal.execution_arn}/*/*"
}

# ── Authorizer ──────────────────────────────────────────────────────────────
resource "aws_apigatewayv2_authorizer" "jwt" {
  api_id          = data.aws_ssm_parameter.apigw_id.value
  name            = "${local.prefixo}-jwt"
  authorizer_type = "REQUEST"
  authorizer_uri  = aws_lambda_function.fn["auth-authorizer"].invoke_arn

  # O handler devolve {isAuthorized, context}. Sem estas duas linhas o
  # Gateway espera uma política IAM completa e nega tudo.
  authorizer_payload_format_version = "2.0"
  enable_simple_responses           = true

  identity_sources = ["$request.header.Authorization"]

  # Sem cache, um teste de carga invoca a Lambda a cada requisição. Com 300 s,
  # o mesmo token reaproveita a decisão. O preço é a janela de revogação:
  # invalidar um token leva até 5 minutos para surtir efeito.
  authorizer_result_ttl_in_seconds = 300
}

resource "aws_lambda_permission" "authorizer" {
  statement_id  = "AllowAPIGatewayInvokeAuthorizer"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.fn["auth-authorizer"].function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${data.aws_apigatewayv2_api.principal.execution_arn}/authorizers/${aws_apigatewayv2_authorizer.jwt.id}"
}

# ── Rotas públicas da aplicação ─────────────────────────────────────────────
resource "aws_apigatewayv2_integration" "publica" {
  for_each = local.rotas_publicas

  api_id                 = data.aws_ssm_parameter.apigw_id.value
  integration_type       = "HTTP_PROXY"
  integration_method     = "ANY"
  integration_uri        = "${var.backend_url}${each.value.caminho}"
  payload_format_version = "1.0" # HTTP_PROXY só aceita 1.0
}

resource "aws_apigatewayv2_route" "publica" {
  for_each = local.rotas_publicas

  api_id    = data.aws_ssm_parameter.apigw_id.value
  route_key = each.value.route_key
  target    = "integrations/${aws_apigatewayv2_integration.publica[each.key].id}"
}

# ── Todo o resto de /v1/* — protegido ───────────────────────────────────────
resource "aws_apigatewayv2_integration" "protegida" {
  api_id                 = data.aws_ssm_parameter.apigw_id.value
  integration_type       = "HTTP_PROXY"
  integration_method     = "ANY"
  integration_uri        = "${var.backend_url}/v1/{proxy}"
  payload_format_version = "1.0"
}

resource "aws_apigatewayv2_route" "protegida" {
  api_id             = data.aws_ssm_parameter.apigw_id.value
  route_key          = "ANY /v1/{proxy+}"
  target             = "integrations/${aws_apigatewayv2_integration.protegida.id}"
  authorization_type = "CUSTOM"
  authorizer_id      = aws_apigatewayv2_authorizer.jwt.id
}

resource "aws_ssm_parameter" "authorizer_id" {
  name  = "/oficina/${var.ambiente}/apigw/authorizer_id"
  type  = "String"
  value = aws_apigatewayv2_authorizer.jwt.id
}
