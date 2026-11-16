//go:build integracao

package consulta

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConsultasPostgres(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL não definida")
	}
	ctx := context.Background()

	// Pool normal só para preparar os dados.
	escrita, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer escrita.Close()
	_, err = escrita.Exec(ctx, `
		TRUNCATE pagamentos, titulos, eventos_processados, outbox;
		INSERT INTO titulos (id, cliente_id, descricao, valor_centavos, pago_centavos, vencimento, status) VALUES
			('t1', 'c1', 'parcial vencido', 10000, 3000,  '2026-11-01', 'parcial'),
			('t2', 'c1', 'a vencer',         5000,    0,  '2026-12-01', 'aberto'),
			('t3', 'c2', 'vencido 30 dias',  8000,    0,  '2026-10-17', 'aberto'),
			('t4', 'c2', 'vencido 6 dias',  12000,    0,  '2026-11-10', 'aberto'),
			('t5', 'c3', 'pago em atraso',   4000, 4000,  '2026-10-01', 'pago');`)
	if err != nil {
		t.Fatal(err)
	}

	pool, err := NovoPoolSomenteLeitura(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	c := NovoPostgres(pool)
	hoje := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)

	t.Run("saldo", func(t *testing.T) {
		s, err := c.Saldo(ctx, "c1", hoje)
		want := Saldo{ClienteID: "c1", TitulosEmAberto: 2, EmAbertoCentavos: 12000, VencidoCentavos: 7000, ProximoVencimento: "2026-12-01"}
		if err != nil || s != want {
			t.Fatalf("saldo c1: %+v err=%v", s, err)
		}
		s, err = c.Saldo(ctx, "c3", hoje)
		if err != nil || s != (Saldo{ClienteID: "c3"}) {
			t.Fatalf("cliente sem nada em aberto: %+v err=%v", s, err)
		}
	})

	t.Run("vencidos", func(t *testing.T) {
		v, err := c.Vencidos(ctx, "", hoje, 50)
		if err != nil || len(v) != 3 {
			t.Fatalf("vencidos: %+v err=%v", v, err)
		}
		if v[0].TituloID != "t3" || v[0].DiasAtraso != 30 || v[1].TituloID != "t1" || v[1].SaldoCentavos != 7000 || v[2].TituloID != "t4" {
			t.Fatalf("ordem ou valores errados: %+v", v)
		}
		if v, _ := c.Vencidos(ctx, "c1", hoje, 50); len(v) != 1 {
			t.Fatalf("filtro por cliente: %+v", v)
		}
		if v, _ := c.Vencidos(ctx, "", hoje, 1); len(v) != 1 {
			t.Fatalf("limite: %+v", v)
		}
	})

	t.Run("inadimplência", func(t *testing.T) {
		l, err := c.Inadimplencia(ctx, hoje, 10)
		want := []Inadimplente{
			{ClienteID: "c2", TitulosVencidos: 2, VencidoCentavos: 20000, MaiorAtrasoDias: 30},
			{ClienteID: "c1", TitulosVencidos: 1, VencidoCentavos: 7000, MaiorAtrasoDias: 15},
		}
		if err != nil || len(l) != 2 || l[0] != want[0] || l[1] != want[1] {
			t.Fatalf("inadimplência: %+v err=%v", l, err)
		}
	})

	t.Run("pool recusa escrita", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE titulos SET descricao = 'x' WHERE id = 't1'`)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "25006" { // read_only_sql_transaction
			t.Fatalf("esperava erro 25006, veio %v", err)
		}
	})
}
