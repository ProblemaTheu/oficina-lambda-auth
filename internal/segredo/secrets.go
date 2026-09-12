// Package segredo lê valores do AWS Secrets Manager com cache em memória.
package segredo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

var (
	mu    sync.Mutex
	cache = map[string]string{}
)

// Buscar devolve o valor bruto do segredo, memorizado por toda a vida do
// container Lambda. Buscar a cada invocação custaria ~30 ms e uma chamada de
// API cobrada — e containers quentes atendem muitas invocações.
func Buscar(ctx context.Context, nome string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if v, ok := cache[nome]; ok {
		return v, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("carregar configuração AWS: %w", err)
	}

	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx,
		&secretsmanager.GetSecretValueInput{SecretId: &nome})
	if err != nil {
		return "", fmt.Errorf("obter segredo %s: %w", nome, err)
	}
	if out.SecretString == nil {
		return "", fmt.Errorf("segredo %s não tem SecretString", nome)
	}

	cache[nome] = *out.SecretString
	return cache[nome], nil
}

// BuscarCampo devolve um campo de um segredo armazenado como JSON.
//
// É o formato que o oficina-app usa em oficina/prod/app:
//
//	{"jwt_secret": "...", "webhook_secret": "..."}
//
// Guardar os dois no mesmo segredo custa uma cobrança em vez de duas e
// mantém o JWT_SECRET literalmente compartilhado entre a aplicação e esta
// Lambda, que é o que a assinatura HS256 exige.
func BuscarCampo(ctx context.Context, nome, campo string) (string, error) {
	bruto, err := Buscar(ctx, nome)
	if err != nil {
		return "", err
	}

	var campos map[string]string
	if err := json.Unmarshal([]byte(bruto), &campos); err != nil {
		return "", fmt.Errorf("segredo %s não é um JSON de strings: %w", nome, err)
	}

	valor, ok := campos[campo]
	if !ok {
		return "", fmt.Errorf("segredo %s não tem o campo %s", nome, campo)
	}
	return valor, nil
}
