// Command auth-token recebe um CPF, valida os dígitos verificadores,
// consulta existência e status do cliente no OS Service e devolve um JWT HS256.
//
// É a "Function que valida o CPF" que o enunciado da Fase 3 pede. Na Fase 4
// (F4-1.5, ADR-015) ela deixou de ler o RDS do OS: pergunta ao dono do dado
// pela rota interna GET /internal/clientes/por-cpf/{cpf}, fora da VPC e sem
// credencial de banco.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ProblemaTheu/oficina-lambda-auth/internal/cliente"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/cpf"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/segredo"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/token"
)

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
}

// Dependências como variáveis de pacote: os testes trocam por fakes sem
// AWS nem rede.
var (
	buscarCliente = buscarClienteNoOS
	chaveJWT      = func(ctx context.Context) (string, error) {
		return segredo.BuscarCampo(ctx, os.Getenv("JWT_SECRET_NAME"), campoDoSegredo())
	}
)

// A Consulta vive FORA do handler: containers Lambda são reaproveitados
// entre invocações, então o pool de conexões HTTP sobrevive. O token de
// serviço vem do Secrets Manager (com cache em memória no pacote segredo).
// Só o sucesso é memorizado: uma falha transitória ao ler o segredo não
// envenena o container — a próxima invocação tenta de novo.
var (
	consultaMu sync.Mutex
	consulta   *cliente.Consulta
)

func buscarClienteNoOS(ctx context.Context, documento string) (cliente.Cliente, error) {
	consultaMu.Lock()
	if consulta == nil {
		tok, err := segredo.BuscarCampo(ctx, os.Getenv("INTERNAL_TOKEN_SECRET_NAME"), "token")
		if err != nil {
			consultaMu.Unlock()
			return cliente.Cliente{}, errors.Join(cliente.ErrIndisponivel, err)
		}
		consulta = cliente.Nova(os.Getenv("OS_INTERNAL_URL"), tok, nil)
	}
	c := consulta
	consultaMu.Unlock()
	return c.PorCPF(ctx, documento)
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

	// 1 — validar o CPF antes de consultar o OS.
	documento, err := cpf.Validar(body.CPF)
	if err != nil {
		// O CPF NUNCA é logado: é dado pessoal (LGPD), e log de autenticação
		// é justamente o que vaza em incidente.
		slog.WarnContext(ctx, "cpf inválido")
		return erro(400, "cpf_invalido", "CPF informado é inválido")
	}

	// 2 — existência E status. O enunciado pede os dois. O OS Service é o
	// dono do cadastro; a Lambda não tem mais rota nem credencial para o RDS.
	cli, err := buscarCliente(ctx, documento)
	switch {
	case errors.Is(err, cliente.ErrNaoEncontrado):
		return erro(404, "cliente_nao_encontrado", "não há cliente cadastrado com este CPF")
	case err != nil:
		slog.ErrorContext(ctx, "falha ao consultar cliente no OS Service", "error", err)
		return erro(503, "servico_indisponivel", "não foi possível consultar o cadastro agora; tente novamente")
	case cli.Status != "ativo":
		slog.WarnContext(ctx, "cliente inativo", "cliente_id", cli.ID, "status", cli.Status)
		return erro(403, "cliente_inativo", "cadastro do cliente não está ativo")
	}
	id, nome := cli.ID, cli.Nome

	// 3 — assinar o token.
	chave, err := chaveJWT(ctx)
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
