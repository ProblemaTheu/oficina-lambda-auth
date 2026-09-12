// Command auth-authorizer valida o JWT na borda, antes que a requisição
// alcance o cluster.
//
// O API Gateway HTTP API tem um JWT Authorizer nativo, mas ele só aceita
// emissores OIDC com JWKS público (Cognito, Auth0). Como a decisão foi HS256
// com segredo compartilhado — ver RFC-003 —, a validação precisa ser um
// authorizer próprio.
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ProblemaTheu/oficina-lambda-auth/internal/segredo"
	tokencontrato "github.com/ProblemaTheu/oficina-lambda-auth/internal/token"
)

// resposta é o formato simples do authorizer: exige
// authorizer_payload_format_version = "2.0" e enable_simple_responses = true
// no Terraform. Sem isso o Gateway espera uma política IAM e rejeita tudo.
type resposta struct {
	IsAuthorized bool              `json:"isAuthorized"`
	Context      map[string]string `json:"context,omitempty"`
}

var negado = resposta{IsAuthorized: false}

// claimsRepassadas seguem para a integração no contexto do authorizer, para
// a aplicação não precisar reparsear o token.
var claimsRepassadas = []string{"sub", "tipo", "cpf", "nome", "papel"}

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
}

func handler(ctx context.Context, ev events.APIGatewayV2CustomAuthorizerV2Request) (resposta, error) {
	// O API Gateway v2 normaliza os nomes de header para minúsculas.
	cabecalho := ev.Headers["authorization"]

	chave, err := segredo.BuscarCampo(ctx, os.Getenv("JWT_SECRET_NAME"), campoDoSegredo())
	if err != nil {
		slog.ErrorContext(ctx, "falha ao obter segredo", "error", err)
		return negado, nil
	}

	r := autorizar(ctx, chave, cabecalho)
	if r.IsAuthorized {
		slog.InfoContext(ctx, "token aceito", "sub", r.Context["sub"], "tipo", r.Context["tipo"])
	}
	return r, nil
}

// autorizar concentra toda a decisão de aceitar ou negar. Separada do
// handler para ser testável sem AWS: é a fronteira de segurança do sistema,
// e o que ela deixa passar chega ao cluster.
func autorizar(ctx context.Context, chave, cabecalho string) resposta {
	if !strings.HasPrefix(cabecalho, "Bearer ") {
		return negado
	}

	t, err := jwt.Parse(
		strings.TrimPrefix(cabecalho, "Bearer "),
		func(*jwt.Token) (any, error) { return []byte(chave), nil },
		// Sem WithValidMethods, um token com header alg=none seria aceito.
		jwt.WithValidMethods([]string{tokencontrato.Metodo}),
		jwt.WithAudience(tokencontrato.Audiencia),
		jwt.WithIssuedAt(),
	)
	if err != nil || !t.Valid {
		slog.WarnContext(ctx, "token rejeitado", "error", err)
		return negado
	}

	claims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return negado
	}

	saida := map[string]string{}
	for _, k := range claimsRepassadas {
		if v, ok := claims[k].(string); ok {
			saida[k] = v
		}
	}

	return resposta{IsAuthorized: true, Context: saida}
}

func campoDoSegredo() string {
	if c := os.Getenv("JWT_SECRET_KEY"); c != "" {
		return c
	}
	return "jwt_secret"
}

func main() { lambda.Start(handler) }
