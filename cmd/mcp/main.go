// mcp é o servidor MCP só leitura (stdio). O assistente inicia o processo e
// conversa por stdin/stdout, então todo log vai para stderr.
//
// Uso no Claude Code: o .mcp.json da raiz já aponta para
// `docker compose run --rm -T mcp`.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // fuso embutido no binário (a imagem distroless não precisa ter)

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LucasMaPaiva/accounts-receivable/internal/consulta"
	"github.com/LucasMaPaiva/accounts-receivable/internal/mcpserver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	brasilia, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		slog.Error("fuso", "erro", err)
		os.Exit(1)
	}
	pool, err := consulta.NovoPoolSomenteLeitura(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("banco", "erro", err)
		os.Exit(1)
	}
	defer pool.Close()

	agora := func() time.Time { return time.Now().In(brasilia) }
	srv := mcpserver.Novo(consulta.NovoPostgres(pool), agora)
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		slog.Error("servidor MCP", "erro", err)
		os.Exit(1)
	}
}
