package cpf_test

import (
	"errors"
	"testing"

	"github.com/ProblemaTheu/oficina-lambda-auth/internal/cpf"
)

func TestValidar(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
		erro     bool
	}{
		{"válido com máscara", "529.982.247-25", "52998224725", false},
		{"válido sem máscara", "52998224725", "52998224725", false},
		{"válido com espaços em volta", "  529.982.247-25  ", "52998224725", false},
		{"outro válido (cliente inativo do seed)", "111.444.777-35", "11144477735", false},
		{"primeiro dígito verificador errado", "52998224715", "", true},
		{"segundo dígito verificador errado", "52998224726", "", true},
		{"todos os dígitos iguais", "11111111111", "", true},
		{"todos zeros", "00000000000", "", true},
		{"curto demais", "1234567890", "", true},
		{"longo demais", "529982247251", "", true},
		{"só letras", "abcdefghijk", "", true},
		{"letra no meio de dígitos válidos", "5299822472a", "", true},
		{"vazio", "", "", true},
		{"só máscara", "...-", "", true},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			obtido, err := cpf.Validar(c.entrada)

			if c.erro {
				if err == nil {
					t.Fatalf("esperava erro, obteve cpf %q", obtido)
				}
				if !errors.Is(err, cpf.ErrInvalido) {
					t.Fatalf("esperava ErrInvalido, obteve %v", err)
				}
				if obtido != "" {
					t.Fatalf("em caso de erro o retorno deve ser vazio, obteve %q", obtido)
				}
				return
			}

			if err != nil {
				t.Fatalf("esperava sucesso, obteve erro %v", err)
			}
			if obtido != c.esperado {
				t.Fatalf("esperava %q, obteve %q", c.esperado, obtido)
			}
		})
	}
}

func TestNormalizar(t *testing.T) {
	casos := map[string]string{
		"529.982.247-25": "52998224725",
		"52998224725":    "52998224725",
		"abc123":         "123",
		"":               "",
	}
	for entrada, esperado := range casos {
		if obtido := cpf.Normalizar(entrada); obtido != esperado {
			t.Errorf("Normalizar(%q) = %q, esperava %q", entrada, obtido, esperado)
		}
	}
}

// A máscara não pode alterar o resultado: é o que garante que CPF digitado
// com e sem pontuação resolva para o mesmo cliente.
func TestValidarIgnoraMascara(t *testing.T) {
	comMascara, err1 := cpf.Validar("529.982.247-25")
	semMascara, err2 := cpf.Validar("52998224725")
	if err1 != nil || err2 != nil {
		t.Fatalf("ambos deveriam ser válidos: %v, %v", err1, err2)
	}
	if comMascara != semMascara {
		t.Fatalf("%q != %q", comMascara, semMascara)
	}
}
