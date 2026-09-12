// Command auth-token recebe um CPF, valida os dígitos verificadores,
// consulta existência e status do cliente no RDS e devolve um JWT HS256.
//
// É a "Function que valida o CPF" que o enunciado da Fase 3 pede.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"

	"github.com/ProblemaTheu/oficina-lambda-auth/internal/cpf"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/segredo"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/token"
)

// db vive FORA do handler: containers Lambda são reaproveitados entre
// invocações, então a conexão sobrevive e poupa o handshake TLS com o RDS.
var db *sql.DB

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	var err error
	db, err = sql.Open("postgres", dsn())
	if err != nil {
		slog.Error("falha ao abrir conexão com o banco", "error", err)
		os.Exit(1)
	}

	// Uma invocação Lambda é single-thread: mais de uma conexão é
	// desperdício e multiplica o consumo do RDS pelo número de containers.
	// Com concorrência alta o certo seria RDS Proxy — ver RFC-003.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)
}

// dsn devolve a DSN do banco garantindo TLS.
//
// O RDS PostgreSQL 15 vem com rds.force_ssl=1 e recusa conexão sem
// criptografia com "no pg_hba.conf entry ... no encryption" — mensagem que
// parece erro de permissão e não é. A rede de segurança evita repetir o
// diagnóstico caso a variável venha sem sslmode.
func dsn() string {
	d := os.Getenv("DB_DSN")
	if d != "" && !strings.Contains(d, "sslmode=") {
		sep := " "
		if strings.HasPrefix(d, "postgres://") || strings.HasPrefix(d, "postgresql://") {
			sep = "?"
			if strings.Contains(d, "?") {
				sep = "&"
			}
		}
		d += sep + "sslmode=require"
	}
	return d
}

type requisicao struct {
	CPF string `json:"cpf"`
}

type resposta struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var body requisicao
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return erro(400, "payload_invalido", "corpo da requisição não é um JSON válido")
	}

	// 1 — validar o CPF antes de tocar no banco.
	documento, err := cpf.Validar(body.CPF)
	if err != nil {
		// O CPF NUNCA é logado: é dado pessoal (LGPD), e log de autenticação
		// é justamente o que vaza em incidente.
		slog.WarnContext(ctx, "cpf inválido")
		return erro(400, "cpf_invalido", "CPF informado é inválido")
	}

	// 2 — existência E status. O enunciado pede os dois.
	var id, nome, status string
	err = db.QueryRowContext(ctx,
		`SELECT id::text, nome, status FROM clientes WHERE cpf_cnpj_digitos = $1`,
		documento,
	).Scan(&id, &nome, &status)

	switch {
	case err == sql.ErrNoRows:
		return erro(404, "cliente_nao_encontrado", "não há cliente cadastrado com este CPF")
	case err != nil:
		slog.ErrorContext(ctx, "falha ao consultar cliente", "error", err)
		return erro(500, "erro_interno", "não foi possível processar a autenticação")
	case status != "ativo":
		slog.WarnContext(ctx, "cliente inativo", "cliente_id", id, "status", status)
		return erro(403, "cliente_inativo", "cadastro do cliente não está ativo")
	}

	// 3 — assinar o token.
	chave, err := segredo.BuscarCampo(ctx, os.Getenv("JWT_SECRET_NAME"), campoDoSegredo())
	if err != nil {
		slog.ErrorContext(ctx, "falha ao obter segredo", "error", err)
		return erro(500, "erro_interno", "não foi possível processar a autenticação")
	}

	agora := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  id,
		"cpf":  documento,
		"nome": nome,
		"tipo": token.TipoCliente,
		"iss":  token.EmissorLambda,
		"aud":  token.Audiencia,
		"iat":  agora.Unix(),
		"exp":  agora.Add(token.Validade).Unix(),
	})

	assinado, err := t.SignedString([]byte(chave))
	if err != nil {
		slog.ErrorContext(ctx, "falha ao assinar token", "error", err)
		return erro(500, "erro_interno", "não foi possível processar a autenticação")
	}

	slog.InfoContext(ctx, "token emitido", "cliente_id", id)
	return json200(resposta{
		AccessToken: assinado,
		TokenType:   "Bearer",
		ExpiresIn:   int(token.Validade.Seconds()),
	})
}

func campoDoSegredo() string {
	if c := os.Getenv("JWT_SECRET_KEY"); c != "" {
		return c
	}
	return "jwt_secret"
}

func json200(v any) (events.APIGatewayV2HTTPResponse, error) {
	corpo, err := json.Marshal(v)
	if err != nil {
		return erro(500, "erro_interno", "falha ao serializar a resposta")
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(corpo),
	}, nil
}

// erro devolve o mesmo formato de erro da aplicação: {code, message}.
func erro(status int, codigo, mensagem string) (events.APIGatewayV2HTTPResponse, error) {
	corpo, _ := json.Marshal(map[string]string{"code": codigo, "message": mensagem})
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(corpo),
	}, nil
}

func main() { lambda.Start(handler) }
