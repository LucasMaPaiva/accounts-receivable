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

	"github.com/LucasMaPaiva/accounts-receivable/internal/api"
	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
)

func main() {
	porta := os.Getenv("PORTA")
	if porta == "" {
		porta = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + porta,
		Handler:           api.NovoHandler(repositorio.NovaMemoria()),
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Desligamento gracioso: termina as requisições em andamento (até 10 s).
	desligar, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(desligar); err != nil {
		slog.Error("falha ao desligar", "erro", err)
	}
	slog.Info("servidor encerrado")
}
