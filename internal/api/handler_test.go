package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

func requisicao(t *testing.T, h http.Handler, metodo, rota, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, rota, strings.NewReader(corpo))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFluxoCompleto(t *testing.T) {
	h := NovoHandler(repositorio.NovaMemoria())

	rec := requisicao(t, h, "POST", "/titulos",
		`{"cliente_id":"c1","descricao":"Mensalidade","valor_centavos":10000,"vencimento":"2026-12-10"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: status %d, corpo %s", rec.Code, rec.Body)
	}
	var criado titulo.Titulo
	_ = json.Unmarshal(rec.Body.Bytes(), &criado)

	rec = requisicao(t, h, "POST", "/titulos/"+criado.ID+"/pagamentos", `{"valor_centavos":10000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("pagar: status %d, corpo %s", rec.Code, rec.Body)
	}

	rec = requisicao(t, h, "GET", "/titulos/"+criado.ID, "")
	var lido titulo.Titulo
	_ = json.Unmarshal(rec.Body.Bytes(), &lido)
	if lido.Status != titulo.StatusPago {
		t.Fatalf("status = %s, esperado pago", lido.Status)
	}
}

func TestErros(t *testing.T) {
	h := NovoHandler(repositorio.NovaMemoria())

	casos := []struct {
		nome, metodo, rota, corpo string
		status                    int
	}{
		{"JSON inválido", "POST", "/titulos", `{`, http.StatusBadRequest},
		{"data inválida", "POST", "/titulos", `{"cliente_id":"c1","valor_centavos":1,"vencimento":"10/12/2026"}`, http.StatusUnprocessableEntity},
		{"valor zero", "POST", "/titulos", `{"cliente_id":"c1","valor_centavos":0,"vencimento":"2026-12-10"}`, http.StatusUnprocessableEntity},
		{"inexistente", "GET", "/titulos/nao-existe", "", http.StatusNotFound},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := requisicao(t, h, c.metodo, c.rota, c.corpo)
			if rec.Code != c.status {
				t.Fatalf("status %d, esperado %d (%s)", rec.Code, c.status, rec.Body)
			}
		})
	}
}
