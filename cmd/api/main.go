package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LucasMaPaiva/accounts-receivable/internal/api"
	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, fechar, err := escolherRepositorio(ctx)
	if err != nil {
		slog.Error("falha ao conectar no banco", "erro", err)
		os.Exit(1)
	}
	defer fechar()

	porta := os.Getenv("PORTA")
	if porta == "" {
		porta = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + porta,
		Handler:           api.NovoHandler(repo),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// O servidor roda numa goroutine; a principal espera o sinal de parada.
	go func() {
		slog.Info("servidor no ar", "porta", porta)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("falha no servidor", "erro", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	// Desligamento gracioso: termina as requisições em andamento (até 10 s).
	desligar, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(desligar); err != nil {
		slog.Error("falha ao desligar", "erro", err)
	}
	slog.Info("servidor encerrado")
}

// Com DATABASE_URL usa o Postgres; sem ela, memória (útil para testar rápido).
func escolherRepositorio(ctx context.Context) (api.Repositorio, func(), error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		slog.Warn("DATABASE_URL vazia: usando repositório em memória")
		return repositorio.NovaMemoria(), func() {}, nil
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, nil, err
	}
	slog.Info("conectado ao PostgreSQL")
	return repositorio.NovoPostgres(pool), pool.Close, nil
}
