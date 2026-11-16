package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ProblemaTheu/oficina-lambda-auth/internal/cliente"
	"github.com/ProblemaTheu/oficina-lambda-auth/internal/token"
)

const chaveTeste = "chave-de-teste-com-mais-de-32-caracteres!!"

// fakes troca as dependências externas e devolve quantas vezes o OS foi
// consultado.
func fakes(t *testing.T, cli cliente.Cliente, err error) *int {
	t.Helper()
	chamadas := 0
	antesCliente, antesChave := buscarCliente, chaveJWT
	buscarCliente = func(context.Context, string) (cliente.Cliente, error) {
		chamadas++
		return cli, err
	}
	chaveJWT = func(context.Context) (string, error) { return chaveTeste, nil }
	t.Cleanup(func() { buscarCliente, chaveJWT = antesCliente, antesChave })
	return &chamadas
}

func invocar(t *testing.T, corpo string) (int, map[string]any) {
	t.Helper()
	resp, err := handler(context.Background(), events.APIGatewayV2HTTPRequest{Body: corpo})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(resp.Body), &m); err != nil {
		t.Fatalf("corpo não é JSON: %s", resp.Body)
	}
	return resp.StatusCode, m
}

// Os 4 cenários da Fase 3 (F3-0.2) continuam iguais com a consulta via OS.
func TestCenariosDaFase3(t *testing.T) {
	ativo := cliente.Cliente{ID: "c1", Nome: "Maria", Status: "ativo"}
	casos := []struct {
		nome       string
		corpo      string
		cli        cliente.Cliente
		err        error
		status     int
		codigo     string
		consultaOS bool
	}{
		{"cpf inválido", `{"cpf":"111.111.111-11"}`, ativo, nil, 400, "cpf_invalido", false},
		{"payload inválido", `{`, ativo, nil, 400, "payload_invalido", false},
		{"cliente inexistente", `{"cpf":"529.982.247-25"}`, cliente.Cliente{}, cliente.ErrNaoEncontrado, 404, "cliente_nao_encontrado", true},
		{"cliente inativo", `{"cpf":"52998224725"}`, cliente.Cliente{ID: "c2", Status: "bloqueado"}, nil, 403, "cliente_inativo", true},
		{"os indisponível", `{"cpf":"52998224725"}`, cliente.Cliente{}, cliente.ErrIndisponivel, 503, "servico_indisponivel", true},
		{"erro inesperado", `{"cpf":"52998224725"}`, cliente.Cliente{}, errors.New("x"), 503, "servico_indisponivel", true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			chamadas := fakes(t, c.cli, c.err)
			status, corpo := invocar(t, c.corpo)
			if status != c.status || corpo["code"] != c.codigo {
				t.Errorf("status = %d, code = %v; quer %d %s", status, corpo["code"], c.status, c.codigo)
			}
			if (*chamadas > 0) != c.consultaOS {
				t.Errorf("consultou o OS %d vez(es)", *chamadas)
			}
		})
	}
}

func TestClienteAtivoRecebeTokenValido(t *testing.T) {
	fakes(t, cliente.Cliente{ID: "c1", Nome: "Maria", Status: "ativo"}, nil)
	status, corpo := invocar(t, `{"cpf":"529.982.247-25"}`)
	if status != 200 || corpo["token_type"] != "Bearer" {
		t.Fatalf("status = %d corpo = %v", status, corpo)
	}
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(corpo["access_token"].(string), claims,
		func(*jwt.Token) (any, error) { return []byte(chaveTeste), nil }); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "c1" || claims["nome"] != "Maria" || claims["cpf"] != "52998224725" || claims["tipo"] != token.TipoCliente {
		t.Errorf("claims = %v", claims)
	}
}

func TestFalhaAoLerChaveJWT(t *testing.T) {
	fakes(t, cliente.Cliente{ID: "c1", Status: "ativo"}, nil)
	chaveJWT = func(context.Context) (string, error) { return "", errors.New("sem acesso") }
	if status, _ := invocar(t, `{"cpf":"52998224725"}`); status != 500 {
		t.Errorf("status = %d, quer 500", status)
	}
}

// Sem acesso ao segredo do token interno: 503 e nada memorizado — a próxima
// invocação tenta de novo.
func TestBuscarClienteNoOS_SemSegredoNaoMemorizaFalha(t *testing.T) {
	t.Setenv("INTERNAL_TOKEN_SECRET_NAME", "")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "x")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "x")
	t.Setenv("AWS_ENDPOINT_URL", "http://127.0.0.1:1")
	_, err := buscarClienteNoOS(context.Background(), "52998224725")
	if !errors.Is(err, cliente.ErrIndisponivel) {
		t.Fatalf("err = %v", err)
	}
	if consulta != nil {
		t.Error("consulta não deveria ter sido criada sem o token")
	}
}
