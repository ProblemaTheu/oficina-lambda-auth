package main

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	tokencontrato "github.com/ProblemaTheu/oficina-lambda-auth/internal/token"
)

const chaveTeste = "segredo-de-teste-com-tamanho-suficiente"

// assinar monta um token com as claims informadas, permitindo sobrescrever
// qualquer uma para exercitar os casos de rejeição.
func assinar(t *testing.T, chave string, metodo jwt.SigningMethod, sobrescrever map[string]any) string {
	t.Helper()

	agora := time.Now()
	claims := jwt.MapClaims{
		"sub":  "770e8400-e29b-41d4-a716-446655440001",
		"cpf":  "52998224725",
		"nome": "Maria Silva",
		"tipo": tokencontrato.TipoCliente,
		"iss":  tokencontrato.EmissorLambda,
		"aud":  tokencontrato.Audiencia,
		"iat":  agora.Unix(),
		"exp":  agora.Add(time.Hour).Unix(),
	}
	for k, v := range sobrescrever {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}

	tok := jwt.NewWithClaims(metodo, claims)

	var (
		assinado string
		err      error
	)
	if metodo == jwt.SigningMethodNone {
		assinado, err = tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	} else {
		assinado, err = tok.SignedString([]byte(chave))
	}
	if err != nil {
		t.Fatalf("falha ao assinar token de teste: %v", err)
	}
	return assinado
}

func TestAutorizar(t *testing.T) {
	ctx := context.Background()
	agora := time.Now()

	casos := []struct {
		nome      string
		cabecalho string
		aceita    bool
	}{
		{
			nome:      "token válido",
			cabecalho: "Bearer " + assinar(t, chaveTeste, jwt.SigningMethodHS256, nil),
			aceita:    true,
		},
		{
			nome:      "sem cabeçalho",
			cabecalho: "",
		},
		{
			nome:      "sem o prefixo Bearer",
			cabecalho: assinar(t, chaveTeste, jwt.SigningMethodHS256, nil),
		},
		{
			nome:      "prefixo em minúsculas",
			cabecalho: "bearer " + assinar(t, chaveTeste, jwt.SigningMethodHS256, nil),
		},
		{
			nome:      "assinado com outro segredo",
			cabecalho: "Bearer " + assinar(t, "outro-segredo-qualquer", jwt.SigningMethodHS256, nil),
		},
		{
			nome: "expirado",
			cabecalho: "Bearer " + assinar(t, chaveTeste, jwt.SigningMethodHS256, map[string]any{
				"iat": agora.Add(-2 * time.Hour).Unix(),
				"exp": agora.Add(-time.Hour).Unix(),
			}),
		},
		{
			nome: "audiência de outra API",
			cabecalho: "Bearer " + assinar(t, chaveTeste, jwt.SigningMethodHS256, map[string]any{
				"aud": "outra-api",
			}),
		},
		{
			nome: "sem audiência",
			cabecalho: "Bearer " + assinar(t, chaveTeste, jwt.SigningMethodHS256, map[string]any{
				"aud": nil,
			}),
		},
		{
			// O ataque clássico: trocar o header para alg=none e mandar o
			// token sem assinatura. É o que WithValidMethods impede.
			nome:      "alg none",
			cabecalho: "Bearer " + assinar(t, chaveTeste, jwt.SigningMethodNone, nil),
		},
		{
			nome:      "lixo no lugar do token",
			cabecalho: "Bearer nao-e-um-jwt",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := autorizar(ctx, chaveTeste, c.cabecalho)
			if r.IsAuthorized != c.aceita {
				t.Fatalf("IsAuthorized = %v, esperava %v", r.IsAuthorized, c.aceita)
			}
			if !c.aceita && len(r.Context) != 0 {
				t.Fatalf("token negado não deve repassar contexto, veio %v", r.Context)
			}
		})
	}
}

// As claims repassadas são o que a aplicação usa para autorizar sem
// reparsear o token — se pararem de chegar, o cliente vira anônimo.
func TestAutorizarRepassaClaims(t *testing.T) {
	r := autorizar(context.Background(), chaveTeste,
		"Bearer "+assinar(t, chaveTeste, jwt.SigningMethodHS256, nil))

	if !r.IsAuthorized {
		t.Fatal("token válido deveria ser aceito")
	}
	esperado := map[string]string{
		"sub":  "770e8400-e29b-41d4-a716-446655440001",
		"cpf":  "52998224725",
		"nome": "Maria Silva",
		"tipo": tokencontrato.TipoCliente,
	}
	for k, v := range esperado {
		if r.Context[k] != v {
			t.Errorf("contexto[%q] = %q, esperava %q", k, r.Context[k], v)
		}
	}
}
