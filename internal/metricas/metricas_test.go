package metricas

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPUsaOPadraoDaRotaNoRotulo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /titulos/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := HTTP(mux)

	for _, id := range []string{"a1", "b2", "c3"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/titulos/"+id, nil))
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/nao-existe", nil))

	// Três ids diferentes, uma série só.
	if n := testutil.ToFloat64(RequisicoesHTTP.WithLabelValues("GET", "/titulos/{id}", "418")); n != 3 {
		t.Fatalf("esperava 3 requisições em /titulos/{id}, veio %v", n)
	}
	if n := testutil.ToFloat64(RequisicoesHTTP.WithLabelValues("GET", "nao_encontrada", "404")); n != 1 {
		t.Fatalf("esperava 1 requisição não encontrada, veio %v", n)
	}
	if n := testutil.CollectAndCount(DuracaoHTTP); n != 2 {
		t.Fatalf("esperava 2 séries de duração, veio %d", n)
	}
}

func TestOutboxPendentes(t *testing.T) {
	falhar := false
	RegistrarOutboxPendentes(func() (int, error) {
		if falhar {
			return 0, errors.New("banco fora")
		}
		return 7, nil
	})

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "contas_outbox_pendentes 7") {
		t.Fatal("gauge da outbox não apareceu em /metrics")
	}

	falhar = true
	rec = httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "contas_outbox_pendentes -1") {
		t.Fatal("falha na consulta deveria aparecer como -1")
	}
}
