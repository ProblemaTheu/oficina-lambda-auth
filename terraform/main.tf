# Funções de autenticação, authorizer e rotas do API Gateway.
#
# As rotas moram aqui, e não no oficina-infra-k8s que cria a API, porque uma
# rota protegida depende da integração E do authorizer ao mesmo tempo — e o
# authorizer é uma destas Lambdas. Com as rotas do lado do authorizer, a
# ordem de provisionamento vira uma linha reta e o infra-k8s é aplicado uma
# única vez.

locals {
  prefixo = "${var.projeto}-${var.ambiente}"

  funcoes = {
    "auth-token"      = { timeout = 10, memoria = 256 }
    "auth-authorizer" = { timeout = 5, memoria = 128 }
  }
}

# ── Contrato com os outros repositórios ─────────────────────────────────────
# Fase 4 (F4-1.5, ADR-015): sem subnets, SG nem segredo do banco — a Lambda
# não tem mais rota nem credencial para o RDS do OS.
data "aws_ssm_parameter" "apigw_id" { name = "/oficina/${var.ambiente}/apigw/id" }
data "aws_ssm_parameter" "jwt_secret_arn" { name = "/oficina/${var.ambiente}/jwt/secret_arn" }
data "aws_ssm_parameter" "apigw_url" { name = "/oficina/${var.ambiente}/apigw/url" }

data "aws_apigatewayv2_api" "principal" {
  api_id = data.aws_ssm_parameter.apigw_id.value
}

# Token de serviço das rotas /internal/* do OS Service. Criado pelo
# scripts/prod-secret.sh do oficina-app no formato {"token": "..."} — o mesmo
# valor vai para o INTERNAL_TOKEN do pod do OS. Só o ARN entra aqui: o valor
# é lido pela função em tempo de execução, não pelo Terraform.
data "aws_secretsmanager_secret" "interno" {
  name = "oficina/${var.ambiente}/internal"
}

# ── IAM ─────────────────────────────────────────────────────────────────────
resource "aws_iam_role" "lambda" {
  name = "${local.prefixo}-lambda-auth"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "basico" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Sem AWSLambdaVPCAccessExecutionRole: a função saiu da VPC (ADR-015).
# Só os dois segredos que as funções realmente leem — nada de "*".
resource "aws_iam_role_policy" "segredos" {
  name = "ler-segredos"
  role = aws_iam_role.lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["secretsmanager:GetSecretValue"]
      Resource = [
        data.aws_ssm_parameter.jwt_secret_arn.value,
        data.aws_secretsmanager_secret.interno.arn,
      ]
    }]
  })
}

# ── Funções ─────────────────────────────────────────────────────────────────
# Os zips vêm de `make build` na raiz do repositório. São gerados, não
# versionados: o CI roda o make antes do terraform.
resource "aws_lambda_function" "fn" {
  for_each = local.funcoes

  function_name = "${local.prefixo}-${each.key}"
  role          = aws_iam_role.lambda.arn

  filename = "${path.module}/../dist/${each.key}.zip"
  # Sem isto o Terraform não percebe que o binário mudou e não redeploya.
  source_code_hash = filebase64sha256("${path.module}/../dist/${each.key}.zip")

  runtime       = "provided.al2023" # runtime custom para Go
  handler       = "bootstrap"       # nome obrigatório do binário no zip
  architectures = ["arm64"]         # Graviton: ~20% mais barato

  timeout     = each.value.timeout
  memory_size = each.value.memoria

  environment {
    variables = {
      JWT_SECRET_NAME = data.aws_ssm_parameter.jwt_secret_arn.value
      JWT_SECRET_KEY  = "jwt_secret"

      # auth-token consulta o cliente no OS Service (F4-1.5). Pelo balanceador
      # público: as duas funções ficam fora da VPC, sem ENI no cold start.
      OS_INTERNAL_URL            = local.backend["os"]
      INTERNAL_TOKEN_SECRET_NAME = data.aws_secretsmanager_secret.interno.arn
    }
  }
}

resource "aws_cloudwatch_log_group" "fn" {
  for_each          = local.funcoes
  name              = "/aws/lambda/${local.prefixo}-${each.key}"
  retention_in_days = 14
}
