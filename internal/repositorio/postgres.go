package repositorio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

// Postgres usa um pool de conexões: várias requisições (goroutines) usam o
// banco ao mesmo tempo, cada uma pega uma conexão livre do pool.
type Postgres struct {
	pool *pgxpool.Pool
}

func NovoPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

const colunas = "id, cliente_id, descricao, valor_centavos, pago_centavos, vencimento, status"

func scanTitulo(row pgx.Row) (*titulo.Titulo, error) {
	var t titulo.Titulo
	err := row.Scan(&t.ID, &t.ClienteID, &t.Descricao, &t.ValorCentavos, &t.PagoCentavos, &t.Vencimento, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrado
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Salvar cria ou atualiza (upsert). O status vai junto porque o banco confere
// se ele é coerente com o valor pago.
func (p *Postgres) Salvar(ctx context.Context, t *titulo.Titulo) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO titulos (`+colunas+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			descricao      = EXCLUDED.descricao,
			valor_centavos = EXCLUDED.valor_centavos,
			pago_centavos  = EXCLUDED.pago_centavos,
			vencimento     = EXCLUDED.vencimento,
			status         = EXCLUDED.status,
			atualizado_em  = now()`,
		t.ID, t.ClienteID, t.Descricao, t.ValorCentavos, t.PagoCentavos, t.Vencimento, t.Status)
	if err != nil {
		return fmt.Errorf("salvar título %s: %w", t.ID, err)
	}
	return nil
}

func (p *Postgres) Buscar(ctx context.Context, id string) (*titulo.Titulo, error) {
	t, err := scanTitulo(p.pool.QueryRow(ctx, `SELECT `+colunas+` FROM titulos WHERE id = $1`, id))
	if err != nil && !errors.Is(err, ErrNaoEncontrado) {
		return nil, fmt.Errorf("buscar título %s: %w", id, err)
	}
	return t, err
}

// Listar monta o WHERE só com os filtros preenchidos, sempre com parâmetros
// ($1, $2...), nunca concatenando valor do usuário no SQL.
func (p *Postgres) Listar(ctx context.Context, f Filtro) ([]titulo.Titulo, error) {
	var cond []string
	var args []any
	add := func(sql string, v any) {
		args = append(args, v)
		cond = append(cond, fmt.Sprintf(sql, len(args)))
	}
	if f.ClienteID != "" {
		add("cliente_id = $%d", f.ClienteID)
	}
	if f.Status != "" {
		add("status = $%d", string(f.Status))
	}
	if !f.VencidosEm.IsZero() {
		// Mesma condição do índice parcial titulos_em_aberto_venc_idx.
		add("vencimento < $%d AND status <> 'pago'", f.VencidosEm)
	}
	sql := `SELECT ` + colunas + ` FROM titulos`
	if len(cond) > 0 {
		sql += " WHERE " + strings.Join(cond, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY vencimento, id LIMIT %d", LimiteListagem)

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("listar títulos: %w", err)
	}
	defer rows.Close()

	lista := []titulo.Titulo{}
	for rows.Next() {
		t, err := scanTitulo(rows)
		if err != nil {
			return nil, fmt.Errorf("listar títulos: %w", err)
		}
		lista = append(lista, *t)
	}
	return lista, rows.Err()
}

// RegistrarPagamento roda numa transação e trava a linha do título
// (SELECT ... FOR UPDATE). Um segundo pagamento simultâneo no mesmo título
// espera o primeiro terminar e já lê o saldo atualizado, então o saldo nunca
// fica negativo. A regra continua no domínio (t.RegistrarPagamento).
func (p *Postgres) RegistrarPagamento(ctx context.Context, id string, valorCentavos int64) (*titulo.Titulo, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transação: %w", err)
	}
	// Se algo der errado antes do Commit, desfaz. Depois do Commit não faz nada.
	defer tx.Rollback(ctx)

	t, err := scanTitulo(tx.QueryRow(ctx, `SELECT `+colunas+` FROM titulos WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, err
	}
	if err := t.RegistrarPagamento(valorCentavos); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO pagamentos (titulo_id, valor_centavos) VALUES ($1, $2)`, id, valorCentavos); err != nil {
		return nil, fmt.Errorf("gravar pagamento: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE titulos SET pago_centavos = $2, status = $3, atualizado_em = now() WHERE id = $1`,
		id, t.PagoCentavos, t.Status); err != nil {
		return nil, fmt.Errorf("atualizar título: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar pagamento: %w", err)
	}
	return t, nil
}
