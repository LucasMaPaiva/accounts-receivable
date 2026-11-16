// Package api expõe o domínio por HTTP usando só a biblioteca padrão
// (net/http com rotas por método, disponível desde o Go 1.22).
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/LucasMaPaiva/accounts-receivable/internal/metricas"
	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

// Repositorio é uma interface implícita: Memoria e Postgres a satisfazem
// sem declarar "implements". O context.Context vem da requisição: se o
// cliente desistir, a consulta no banco é cancelada junto.
type Repositorio interface {
	Salvar(ctx context.Context, t *titulo.Titulo) error
	Buscar(ctx context.Context, id string) (*titulo.Titulo, error)
	Listar(ctx context.Context, f repositorio.Filtro) ([]titulo.Titulo, error)
	RegistrarPagamento(ctx context.Context, id string, valorCentavos int64) (*titulo.Titulo, error)
}

type Handler struct {
	repo  Repositorio
	agora func() time.Time // trocável nos testes
}

func NovoHandler(repo Repositorio) http.Handler {
	h := &Handler{repo: repo, agora: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /titulos", h.criar)
	mux.HandleFunc("GET /titulos", h.listar)
	mux.HandleFunc("GET /titulos/{id}", h.buscar)
	mux.HandleFunc("POST /titulos/{id}/pagamentos", h.pagar)
	mux.HandleFunc("GET /saude", func(w http.ResponseWriter, _ *http.Request) {
		responder(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Na mesma porta para simplificar; em produção ficaria numa porta interna.
	mux.Handle("GET /metrics", metricas.Handler())
	return metricas.HTTP(mux)
}

type criarRequest struct {
	ClienteID     string `json:"cliente_id"`
	Descricao     string `json:"descricao"`
	ValorCentavos int64  `json:"valor_centavos"`
	Vencimento    string `json:"vencimento"` // AAAA-MM-DD
}

func (h *Handler) criar(w http.ResponseWriter, r *http.Request) {
	var req criarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		erro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	venc, err := time.Parse("2006-01-02", req.Vencimento)
	if err != nil {
		erro(w, http.StatusUnprocessableEntity, "vencimento deve estar no formato AAAA-MM-DD")
		return
	}
	t, err := titulo.Novo(novoID(), req.ClienteID, req.Descricao, req.ValorCentavos, venc)
	if err != nil {
		erro(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := h.repo.Salvar(r.Context(), t); err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusCreated, t)
}

// GET /titulos?cliente_id=c1&status=aberto&vencidos=true
func (h *Handler) listar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repositorio.Filtro{ClienteID: q.Get("cliente_id")}
	switch s := titulo.Status(q.Get("status")); s {
	case "", titulo.StatusAberto, titulo.StatusParcial, titulo.StatusPago:
		f.Status = s
	default:
		erro(w, http.StatusUnprocessableEntity, "status deve ser aberto, parcial ou pago")
		return
	}
	if q.Get("vencidos") == "true" {
		f.VencidosEm = h.agora()
	}
	lista, err := h.repo.Listar(r.Context(), f)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"titulos": lista})
}

func (h *Handler) buscar(w http.ResponseWriter, r *http.Request) {
	t, err := h.repo.Buscar(r.Context(), r.PathValue("id"))
	if errors.Is(err, repositorio.ErrNaoEncontrado) {
		erro(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, t)
}

type pagarRequest struct {
	ValorCentavos int64 `json:"valor_centavos"`
}

func (h *Handler) pagar(w http.ResponseWriter, r *http.Request) {
	var req pagarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		erro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	t, err := h.repo.RegistrarPagamento(r.Context(), r.PathValue("id"), req.ValorCentavos)
	switch {
	case err == nil:
		metricas.Pagamentos.WithLabelValues("aceito").Inc()
		responder(w, http.StatusOK, t)
	case errors.Is(err, repositorio.ErrNaoEncontrado):
		erro(w, http.StatusNotFound, err.Error())
	case errors.Is(err, titulo.ErrValorInvalido),
		errors.Is(err, titulo.ErrJaPago),
		errors.Is(err, titulo.ErrPagamentoExcede):
		metricas.Pagamentos.WithLabelValues("recusado").Inc()
		erro(w, http.StatusUnprocessableEntity, err.Error())
	default:
		falhaInterna(w, r, err)
	}
}

func responder(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

func erro(w http.ResponseWriter, status int, msg string) {
	responder(w, status, map[string]string{"erro": msg})
}

// falhaInterna registra o detalhe no log e devolve uma mensagem genérica:
// erro de banco não vaza para o cliente.
func falhaInterna(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("falha interna", "metodo", r.Method, "rota", r.URL.Path, "erro", err)
	erro(w, http.StatusInternalServerError, "erro interno")
}

func novoID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
