// Package cpf valida CPFs brasileiros pelo algoritmo dos dígitos
// verificadores, sem dependência externa.
//
// A mesma lógica existe em oficina-app (internal/domain/valueobject). A
// duplicação é deliberada: os dois têm ciclo de vida e deploy independentes,
// e uma biblioteca compartilhada acoplaria o release de um ao do outro por
// 40 linhas de aritmética que não mudam desde 1998. Ver ADR-013.
package cpf

import (
	"errors"
	"strings"
	"unicode"
)

var ErrInvalido = errors.New("cpf inválido")

// Normalizar remove máscara e devolve apenas os dígitos.
func Normalizar(entrada string) string {
	var b strings.Builder
	for _, r := range entrada {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Validar confere tamanho, repetição e os dois dígitos verificadores.
// Devolve o CPF normalizado quando válido.
func Validar(entrada string) (string, error) {
	d := Normalizar(entrada)
	if len(d) != 11 {
		return "", ErrInvalido
	}
	// CPFs de dígito repetido (111.111.111-11) passam no cálculo dos
	// verificadores, mas são inválidos por convenção da Receita.
	if strings.Count(d, string(d[0])) == 11 {
		return "", ErrInvalido
	}
	if digito(d, 9) != int(d[9]-'0') || digito(d, 10) != int(d[10]-'0') {
		return "", ErrInvalido
	}
	return d, nil
}

// digito calcula o verificador da posição n — 9 para o primeiro, 10 para o
// segundo. Os pesos decrescem a partir de n+1.
func digito(d string, n int) int {
	soma, peso := 0, n+1
	for i := 0; i < n; i++ {
		soma += int(d[i]-'0') * peso
		peso--
	}
	resto := soma * 10 % 11
	if resto == 10 {
		return 0
	}
	return resto
}
