// Package consulta tem as consultas só leitura (saldo, vencidos e
// inadimplência) usadas pelo servidor MCP. Fica separado do repositório de
// escrita: outra conexão, outras necessidades, nenhuma regra de negócio.
package consulta

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Saldo resume a situação de um cliente.
type Saldo struct {
	ClienteID         string `json:"cliente_id"`
	TitulosEmAberto   int    `json:"titulos_em_aberto"`
	EmAbertoCentavos  int64  `json:"em_aberto_centavos"`
	VencidoCentavos   int64  `json:"vencido_centavos"`
	ProximoVencimento string `json:"proximo_vencimento,omitempty"` // AAAA-MM-DD, entre os que ainda não venceram
}

// Vencido é um título não pago com vencimento antes de hoje.
type Vencido struct {
	TituloID      string `json:"titulo_id"`
	ClienteID     string `json:"cliente_id"`
	Descricao     string `json:"descricao"`
	SaldoCentavos int64  `json:"saldo_centavos"`
	Vencimento    string `json:"vencimento"`
	DiasAtraso    int    `json:"dias_atraso"`
}

// Inadimplente agrupa os vencidos de um cliente.
type Inadimplente struct {
	ClienteID       string `json:"cliente_id"`
	TitulosVencidos int    `json:"titulos_vencidos"`
	VencidoCentavos int64  `json:"vencido_centavos"`
	MaiorAtrasoDias int    `json:"maior_atraso_dias"`
}

// NovoPoolSomenteLeitura abre um pool em que toda transação é READ ONLY: o
// próprio Postgres recusa INSERT, UPDATE e DELETE, mesmo que o código erre.
// Em produção o ideal é somar a isso um usuário do banco só com SELECT.
func NovoPoolSomenteLeitura(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuração do banco: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 4
	return pgxpool.NewWithConfig(ctx, cfg)
}

type Postgres struct {
	pool *pgxpool.Pool
}

func NovoPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

const formatoData = "2006-01-02"

// Todas as consultas recebem "hoje" de fora: o teste controla a data e o
// servidor decide o fuso (o dia vira às 21h no horário de Brasília em UTC).

func (p *Postgres) Saldo(ctx context.Context, clienteID string, hoje time.Time) (Saldo, error) {
	s := Saldo{ClienteID: clienteID}
	var proximo *time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT count(*),
		       COALESCE(sum(valor_centavos - pago_centavos), 0),
		       COALESCE(sum(valor_centavos - pago_centavos) FILTER (WHERE vencimento < $2), 0),
		       min(vencimento) FILTER (WHERE vencimento >= $2)
		FROM titulos
		WHERE cliente_id = $1 AND status <> 'pago'`, clienteID, hoje).
		Scan(&s.TitulosEmAberto, &s.EmAbertoCentavos, &s.VencidoCentavos, &proximo)
	if err != nil {
		return Saldo{}, fmt.Errorf("saldo do cliente %s: %w", clienteID, err)
	}
	if proximo != nil {
		s.ProximoVencimento = proximo.Format(formatoData)
	}
	return s, nil
}

// Vencidos lista os títulos vencidos, do mais antigo para o mais novo.
// clienteID vazio traz de todos os clientes. Usa o índice parcial
// titulos_em_aberto_venc_idx.
func (p *Postgres) Vencidos(ctx context.Context, clienteID string, hoje time.Time, limite int) ([]Vencido, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, cliente_id, descricao, valor_centavos - pago_centavos, vencimento, $1::date - vencimento
		FROM titulos
		WHERE status <> 'pago' AND vencimento < $1 AND ($2 = '' OR cliente_id = $2)
		ORDER BY vencimento, id
		LIMIT $3`, hoje, clienteID, limite)
	if err != nil {
		return nil, fmt.Errorf("títulos vencidos: %w", err)
	}
	defer rows.Close()

	lista := []Vencido{} // nunca nil: vira [] no JSON, não null
	for rows.Next() {
		var v Vencido
		var venc time.Time
		if err := rows.Scan(&v.TituloID, &v.ClienteID, &v.Descricao, &v.SaldoCentavos, &venc, &v.DiasAtraso); err != nil {
			return nil, fmt.Errorf("títulos vencidos: %w", err)
		}
		v.Vencimento = venc.Format(formatoData)
		lista = append(lista, v)
	}
	return lista, rows.Err()
}

// Inadimplencia ordena os clientes pelo valor vencido, do maior para o menor.
func (p *Postgres) Inadimplencia(ctx context.Context, hoje time.Time, limite int) ([]Inadimplente, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT cliente_id, count(*), sum(valor_centavos - pago_centavos), max($1::date - vencimento)
		FROM titulos
		WHERE status <> 'pago' AND vencimento < $1
		GROUP BY cliente_id
		ORDER BY 3 DESC, cliente_id
		LIMIT $2`, hoje, limite)
	if err != nil {
		return nil, fmt.Errorf("inadimplência: %w", err)
	}
	defer rows.Close()

	lista := []Inadimplente{}
	for rows.Next() {
		var i Inadimplente
		if err := rows.Scan(&i.ClienteID, &i.TitulosVencidos, &i.VencidoCentavos, &i.MaiorAtrasoDias); err != nil {
			return nil, fmt.Errorf("inadimplência: %w", err)
		}
		lista = append(lista, i)
	}
	return lista, rows.Err()
}
