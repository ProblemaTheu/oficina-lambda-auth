package cliente

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func servidor(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func TestPorCPF_Sucesso_EnviaTokenECaminho(t *testing.T) {
	s := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/clientes/por-cpf/52998224725" {
			t.Errorf("caminho = %s", r.URL.Path)
		}
		if r.Header.Get(HeaderToken) != "segredo" {
			t.Errorf("token = %q", r.Header.Get(HeaderToken))
		}
		_, _ = w.Write([]byte(`{"id":"abc","nome":"Maria","status":"ativo"}`))
	})

	cli, err := Nova(s.URL+"/", "segredo", nil).PorCPF(context.Background(), "52998224725")
	if err != nil {
		t.Fatal(err)
	}
	if cli != (Cliente{ID: "abc", Nome: "Maria", Status: "ativo"}) {
		t.Errorf("cliente = %+v", cli)
	}
}

func TestPorCPF_404ViraNaoEncontrado(t *testing.T) {
	s := servidor(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	_, err := Nova(s.URL, "t", nil).PorCPF(context.Background(), "52998224725")
	if !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("err = %v", err)
	}
}

func TestPorCPF_FalhasViramIndisponivel(t *testing.T) {
	casos := map[string]http.HandlerFunc{
		"5xx":            func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		"401 inesperado": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"json inválido":  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) },
		"sem id":         func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ativo"}`)) },
		"sem status":     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":"x"}`)) },
	}
	for nome, h := range casos {
		t.Run(nome, func(t *testing.T) {
			s := servidor(t, h)
			_, err := Nova(s.URL, "t", nil).PorCPF(context.Background(), "52998224725")
			if !errors.Is(err, ErrIndisponivel) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// O erro de rede do net/http carrega a URL, que carrega o CPF: não pode
// subir para o log (LGPD).
func TestPorCPF_ErroDeRedeNaoExpoeOCPF(t *testing.T) {
	_, err := Nova("http://127.0.0.1:1", "t", nil).PorCPF(context.Background(), "52998224725")
	if !errors.Is(err, ErrIndisponivel) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "52998224725") {
		t.Errorf("CPF vazou no erro: %v", err)
	}
}

func TestPorCPF_Timeout(t *testing.T) {
	s := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	inicio := time.Now()
	_, err := Nova(s.URL, "t", &http.Client{Timeout: 50 * time.Millisecond}).PorCPF(context.Background(), "52998224725")
	if !errors.Is(err, ErrIndisponivel) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(inicio) > 2*time.Second {
		t.Error("não respeitou o timeout")
	}
}

func TestPorCPF_URLBaseInvalida(t *testing.T) {
	_, err := Nova("://sem-esquema", "t", nil).PorCPF(context.Background(), "52998224725")
	if !errors.Is(err, ErrIndisponivel) {
		t.Fatalf("err = %v", err)
	}
}
