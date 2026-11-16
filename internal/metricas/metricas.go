// Package metricas define as métricas Prometheus do serviço. Técnicas
// (requisições, latência) e de negócio (pagamentos, eventos, outbox), para
// responder "está tudo bem?" sem abrir o log.
package metricas

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequisicoesHTTP = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contas_http_requisicoes_total",
		Help: "Requisições HTTP por método, rota e status.",
	}, []string{"metodo", "rota", "status"})

	DuracaoHTTP = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "contas_http_duracao_segundos",
		Help:    "Tempo de resposta por método e rota.",
		Buckets: prometheus.DefBuckets,
	}, []string{"metodo", "rota"})

	// resultado: aceito | recusado (regra de negócio)
	Pagamentos = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contas_pagamentos_total",
		Help: "Pagamentos recebidos pela API.",
	}, []string{"resultado"})

	// resultado: criado | repetido | invalido | erro_temporario
	EventosConsumidos = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contas_eventos_consumidos_total",
		Help: "Mensagens venda.faturada tratadas pelo worker.",
	}, []string{"resultado"})

	OutboxPublicados = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contas_outbox_publicados_total",
		Help: "Eventos da outbox confirmados pelo RabbitMQ.",
	})
)

// RegistrarOutboxPendentes cria um gauge calculado na hora da coleta. Se
// esse número só cresce, o relay parou (RabbitMQ fora, por exemplo): é o
// alerta mais útil do worker.
func RegistrarOutboxPendentes(contar func() (int, error)) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "contas_outbox_pendentes",
		Help: "Eventos gravados na outbox e ainda não publicados (-1 se a consulta falhar).",
	}, func() float64 {
		n, err := contar()
		if err != nil {
			return -1
		}
		return float64(n)
	})
}

// Handler expõe /metrics no formato do Prometheus.
func Handler() http.Handler {
	return promhttp.Handler()
}

// HTTP mede cada requisição. A rota é o padrão do ServeMux
// ("/titulos/{id}"), não o caminho real ("/titulos/abc123"): com o id no
// rótulo, cada título viraria uma série nova e a memória do Prometheus explode.
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		// O ServeMux preenche r.Pattern ao escolher a rota.
		rota := "nao_encontrada"
		if _, caminho, ok := strings.Cut(r.Pattern, " "); ok {
			rota = caminho
		} else if r.Pattern != "" {
			rota = r.Pattern
		}
		RequisicoesHTTP.WithLabelValues(r.Method, rota, strconv.Itoa(sw.status)).Inc()
		DuracaoHTTP.WithLabelValues(r.Method, rota).Observe(time.Since(inicio).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
