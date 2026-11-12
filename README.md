# oficina-lambda-auth

> Autenticação **serverless por CPF** da oficina: duas AWS Lambda em Go que emitem e validam o JWT usado pelas APIs protegidas.

Parte do Tech Challenge — Fases 3 e 4. Repositórios irmãos: [oficina-app](https://github.com/ProblemaTheu/oficina-app) (OS Service) · [oficina-billing](https://github.com/ProblemaTheu/oficina-billing) · [oficina-execution](https://github.com/ProblemaTheu/oficina-execution) · [oficina-infra-k8s](https://github.com/ProblemaTheu/oficina-infra-k8s) · [oficina-infra-db](https://github.com/ProblemaTheu/oficina-infra-db)

> **Fase 4 (F4-1.5, ADR-015):** a `auth-token` **não lê mais o banco do OS** — "nenhum serviço pode acessar diretamente o banco de outro serviço". Ela consulta o OS Service pela rota interna `GET /internal/clientes/por-cpf/{cpf}` (header `X-Internal-Token`), saiu da VPC e perdeu o DSN. O Gateway passou a rotear também o Billing e o Execution (F4-6.5).

## Papel na arquitetura

```mermaid
sequenceDiagram
    actor C as Cliente
    participant GW as API Gateway
    participant T as λ auth-token
    participant A as λ auth-authorizer
    participant OS as OS Service
    C->>GW: POST /auth/token {cpf}
    GW->>T: invoke
    T->>T: valida dígitos verificadores
    T->>OS: GET /internal/clientes/por-cpf/{cpf} (X-Internal-Token, timeout 2 s)
    T-->>C: 200 {access_token} · 400 inválido · 404 inexistente · 403 inativo · 503 OS indisponível
    C->>GW: GET /v1/... (Bearer)
    GW->>A: authorizer (cache 300 s)
    A-->>GW: allow/deny — 401 não chega ao cluster
```

| Função | Responsabilidade |
|---|---|
| `auth-token` | Valida o CPF, consulta existência **e status** do cliente no OS Service, emite JWT HS256 (1 h) |
| `auth-authorizer` | Valida assinatura, `exp`, `iss` e `aud` na borda, antes de qualquer chamada ao cluster |

## Tecnologias

| Tecnologia | Versão | Uso |
|---|---|---|
| Go | 1.26 | Linguagem |
| `provided.al2023` / arm64 | — | Runtime Lambda (Graviton: mais barato e rápido) |
| golang-jwt/v5 | — | Emissão e validação HS256 |
| Terraform | 1.15.7 | Funções, IAM e rotas no API Gateway |

## Estrutura

```
cmd/auth-token/        handler de emissão do token
cmd/auth-authorizer/   handler do authorizer do API Gateway
internal/cpf/          validação de dígitos verificadores (sem dependência externa)
internal/token/        contrato de claims compartilhado com a aplicação
internal/segredo/      Secrets Manager com cache por container
internal/cliente/      consulta ao OS Service pela rota interna (timeout 2 s)
terraform/             funções, IAM, integração e rotas; backends.tf descobre os CLBs
Makefile               empacota as funções em dist/*.zip
```

## Empacotamento

```bash
make build   # dist/auth-token.zip e dist/auth-authorizer.zip
make test    # go test ./... -race -cover
```

O runtime `provided.al2023` tem duas exigências que **falham em silêncio** quando erradas: o binário dentro do zip precisa se chamar `bootstrap`, e a arquitetura precisa casar com a declarada na função — aqui, `arm64`.

## Execução local

```bash
go test ./...        # a validação de CPF é lógica pura e tem tabela de casos
go vet ./... && golangci-lint run
```

## Build e deploy

O binário **precisa** se chamar `bootstrap` no runtime `provided.al2023`:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
  -o dist/auth-token/bootstrap ./cmd/auth-token
```

`.github/workflows/ci-cd.yml`:

| Evento | Jobs |
|---|---|
| PR para `homolog` ou `main` | `gofmt`, `go vet`, build, testes com `-race`, `gitleaks`, e `terraform plan` comentado no PR (role `gha-oficina-lambda-auth-plan`, só leitura) |
| push na `main` / disparo manual | testes → `make build` → `terraform apply` no *environment* `prod` → smoke test: `POST /auth/token` com CPF inválido tem que responder `400` |

O deploy do código é feito pelo Terraform (`source_code_hash`), não por `aws lambda update-function-code` — misturar os dois gera drift permanente no plan. Por isso o PR também compila: sem os zips o plan nem começa, e como o build é reproduzível (`-trimpath`), código igual dá hash igual e o plan mostra `0 to change`.

## Contrato com os outros repositórios

**Consome:** `/oficina/{env}/apigw/id`, `/oficina/{env}/apigw/url`, `/oficina/{env}/jwt/secret_arn` e o segredo `oficina/{env}/internal` (token das rotas internas, criado pelo `scripts/prod-secret.sh` do `oficina-app`)

**Publica:** `/oficina/{env}/apigw/authorizer_id`

### Rotas do API Gateway

| Rota | Backend | Authorizer |
|---|---|---|
| `POST /auth/token` | λ `auth-token` | não |
| `POST /v1/auth/login`, `GET /v1/work-orders/{id}/status`, `POST /v1/webhooks/budget-response`, `GET /health[/…]` | CLB do OS | não |
| `ANY /v1/{proxy+}` | CLB do OS | **sim** |
| `ANY /billing/v1/{proxy+}` | CLB do Billing | **sim** |
| `POST /billing/webhooks/mercadopago` | CLB do Billing | não — validação por `x-signature` no Billing |
| `ANY /execution/v1/{proxy+}` | CLB do Execution | **sim** |
| `/internal/*` | — | **sem rota**: só serviço → serviço |

**Balanceadores:** criados pelo Kubernetes, não pelo Terraform. O `backends.tf` os encontra pela tag `kubernetes.io/service-name=<namespace>/<service>` que o EKS põe em cada CLB. Na Fase 3 o DNS ficava fixo num default e quebrava a cada recriação. Por isso este repositório é aplicado **depois do primeiro deploy dos três serviços**; se faltar algum, o plan falha dizendo qual. Para forçar uma URL: `-var 'backend_urls={os="http://..."}'`.

## Dockerfile

Não aplicável: as funções são empacotadas em **zip** para o runtime `provided.al2023`, mais leve e com cold start menor que imagem de container.

## Segurança

- O CPF **nunca** é registrado em log (dado pessoal — LGPD), nem no erro de rede da consulta ao OS (a URL carrega o CPF)
- Sem credencial nem rota de rede para o RDS: a função não está na VPC e a role só lê os segredos do JWT e do token interno
- Token interno errado faz o OS responder 404 (não revela a rota) — aqui isso aparece como "cliente não encontrado" para um CPF que existe. É o sintoma de token desalinhado entre `oficina/prod/internal` e o Secret do pod do OS
- O segredo do JWT vem do Secrets Manager, com cache por container
- `jwt.WithValidMethods(["HS256"])` impede o ataque `alg=none`

## Documentação

Contrato de claims e decisões de autenticação em [oficina-app/docs](https://github.com/ProblemaTheu/oficina-app/tree/main/docs).
