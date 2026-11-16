// Worker: consome venda.faturada (cria títulos) e publica os eventos da
// outbox (titulo.pago). Roda separado da API e pode ter várias réplicas.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LucasMaPaiva/accounts-receivable/internal/mensageria"
	"github.com/LucasMaPaiva/accounts-receivable/internal/metricas"
	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := rodar(); err != nil {
		slog.Error("worker encerrado com erro", "erro", err)
		os.Exit(1)
	}
}

func rodar() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	metricas.RegistrarOutboxPendentes(func() (int, error) {
		consulta, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var n int
		err := pool.QueryRow(consulta, `SELECT count(*) FROM outbox WHERE publicado_em IS NULL`).Scan(&n)
		return n, err
	})
	srvMetricas := servidorMetricas()
	defer srvMetricas.Close()

	conn, err := mensageria.Conectar(ctx, os.Getenv("RABBITMQ_URL"))
	if err != nil {
		return err
	}
	defer conn.Close()

	// Um canal por tarefa: consumo e publicação não disputam o mesmo canal.
	chConsumo, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := mensageria.DeclararTopologia(chConsumo); err != nil {
		return err
	}
	chPublicacao, err := conn.Channel()
	if err != nil {
		return err
	}

	// Se qualquer uma das goroutines falhar, cancela a outra e encerra.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	erros := make(chan error, 2)
	go func() {
		erros <- mensageria.Consumir(ctx, chConsumo, repositorio.NovoPostgres(pool), novoID)
	}()
	go func() {
		erros <- mensageria.RelayOutbox(ctx, pool, chPublicacao, time.Second)
	}()
	slog.Info("worker no ar")

	err = <-erros
	cancel()
	if err2 := <-erros; err == nil {
		err = err2
	}
	slog.Info("worker encerrado")
	return err
}

// O worker não tem API; abre uma porta só para o Prometheus coletar.
func servidorMetricas() *http.Server {
	porta := os.Getenv("METRICAS_PORTA")
	if porta == "" {
		porta = "9091"
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metricas.Handler())
	srv := &http.Server{Addr: ":" + porta, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor de métricas", "erro", err)
		}
	}()
	return srv
}

func novoID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
