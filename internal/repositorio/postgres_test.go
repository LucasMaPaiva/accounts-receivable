//go:build integracao

// Testes contra um Postgres real. Rodam com: make test-integracao
package repositorio

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgres(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL não definida")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `TRUNCATE pagamentos, titulos, eventos_processados, outbox`); err != nil {
		t.Fatal(err)
	}

	repo := NovoPostgres(pool)
	testarContrato(t, repo)

	// Só o Postgres guarda o histórico: um registro por pagamento aceito.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pagamentos WHERE titulo_id = 'a3'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("histórico de pagamentos do a3: %d linhas, esperado 5", n)
	}

	// Outbox: um único titulo.pago para cada título quitado (a3 e l3), mesmo
	// com 10 tentativas simultâneas no a3; nada para o a2, que ficou parcial.
	var quitados string
	if err := pool.QueryRow(ctx, `
		SELECT string_agg(payload->>'titulo_id', ',' ORDER BY payload->>'titulo_id')
		FROM outbox WHERE tipo = 'titulo.pago'`).Scan(&quitados); err != nil {
		t.Fatal(err)
	}
	if quitados != "a3,l3" {
		t.Fatalf("outbox titulo.pago: %q, esperado \"a3,l3\"", quitados)
	}
}
