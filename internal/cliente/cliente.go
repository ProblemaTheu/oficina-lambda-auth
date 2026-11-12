// Package cliente consulta o cadastro do cliente no OS Service pela rota
// interna GET /internal/clientes/por-cpf/{cpf} (F4-1.5, ADR-015).
//
// Na Fase 3 a auth-token lia a tabela clientes direto no RDS do OS. Na Fase 4
// nenhum componente lê o banco de outro: a Lambda saiu da VPC, perdeu o DSN e
// pergunta ao dono do dado.
package cliente

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HeaderToken é o header que a rota interna exige (mesmo nome no OS Service).
const HeaderToken = "X-Internal-Token"

// Timeout total da consulta. A função tem 10 s; se o OS não responde em 2 s
// o cliente recebe 503 rápido em vez de esperar o timeout da Lambda.
const Timeout = 2 * time.Second

var (
	// ErrNaoEncontrado: não há cliente com o CPF (404 do OS).
	ErrNaoEncontrado = errors.New("cliente não encontrado")
	// ErrIndisponivel: o OS não respondeu, respondeu 5xx, outro status
	// inesperado ou um corpo inválido.
	//
	// Atenção: com o token errado o OS responde 404 de propósito (não revela
	// a rota), então um token desalinhado aparece aqui como "cliente não
	// encontrado". O diagnóstico é o log "cliente não encontrado" para um CPF
	// que existe — ver o README.
	ErrIndisponivel = errors.New("os service indisponível")
)

// Cliente é o resumo devolvido pelo OS: só o que a autenticação usa.
type Cliente struct {
	ID     string `json:"id"`
	Nome   string `json:"nome"`
	Status string `json:"status"`
}

// Consulta faz as chamadas ao OS Service.
type Consulta struct {
	base  string
	token string
	http  *http.Client
}

// Nova devolve uma Consulta para a URL base do OS (ex.: http://<clb>) e o
// token de serviço. httpClient nil usa um cliente com Timeout.
func Nova(base, token string, httpClient *http.Client) *Consulta {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: Timeout}
	}
	return &Consulta{base: strings.TrimRight(base, "/"), token: token, http: httpClient}
}

// PorCPF busca o cliente pelo CPF só com dígitos (já validado pelo chamador).
func (c *Consulta) PorCPF(ctx context.Context, cpf string) (Cliente, error) {
	ctx, cancelar := context.WithTimeout(ctx, Timeout)
	defer cancelar()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/internal/clientes/por-cpf/"+url.PathEscape(cpf), nil)
	if err != nil {
		return Cliente{}, fmt.Errorf("%w: montar requisição: %v", ErrIndisponivel, err)
	}
	req.Header.Set(HeaderToken, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// O erro do net/http traz a URL — que traz o CPF. Não propagar.
		return Cliente{}, fmt.Errorf("%w: falha de rede ou timeout", ErrIndisponivel)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Cliente{}, ErrNaoEncontrado
	case resp.StatusCode != http.StatusOK:
		return Cliente{}, fmt.Errorf("%w: status %d", ErrIndisponivel, resp.StatusCode)
	}

	var cli Cliente
	if err := json.NewDecoder(resp.Body).Decode(&cli); err != nil || cli.ID == "" || cli.Status == "" {
		return Cliente{}, fmt.Errorf("%w: resposta inválida", ErrIndisponivel)
	}
	return cli, nil
}
