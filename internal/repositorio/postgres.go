package repositorio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LucasMaPaiva/accounts-receivable/internal/eventos"
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

const colunas = "id, cliente_id, descricao, valor_centavos, pago_centavos, vencimento, status, venda_id"

func scanTitulo(row pgx.Row) (*titulo.Titulo, error) {
	var t titulo.Titulo
	var vendaID *string // coluna pode ser NULL
	err := row.Scan(&t.ID, &t.ClienteID, &t.Descricao, &t.ValorCentavos, &t.PagoCentavos, &t.Vencimento, &t.Status, &vendaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNaoEncontrado
	}
	if err != nil {
		return nil, err
	}
	if vendaID != nil {
		t.VendaID = *vendaID
	}
	return &t, nil
}

// Salvar cria ou atualiza (upsert). O status vai junto porque o banco confere
// se ele é coerente com o valor pago.
const sqlUpsert = `
	INSERT INTO titulos (` + colunas + `)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (id) DO UPDATE SET
		descricao      = EXCLUDED.descricao,
		valor_centavos = EXCLUDED.valor_centavos,
		pago_centavos  = EXCLUDED.pago_centavos,
		vencimento     = EXCLUDED.vencimento,
		status         = EXCLUDED.status,
		atualizado_em  = now()`

func argsTitulo(t *titulo.Titulo) []any {
	var vendaID *string
	if t.VendaID != "" {
		vendaID = &t.VendaID
	}
	return []any{t.ID, t.ClienteID, t.Descricao, t.ValorCentavos, t.PagoCentavos, t.Vencimento, t.Status, vendaID}
}

func (p *Postgres) Salvar(ctx context.Context, t *titulo.Titulo) error {
	if _, err := p.pool.Exec(ctx, sqlUpsert, argsTitulo(t)...); err != nil {
		return fmt.Errorf("salvar título %s: %w", t.ID, err)
	}
	return nil
}

// CriarDeEvento registra o evento e cria o título na mesma transação.
// Se o evento já existe (entrega repetida), não cria nada e devolve false.
func (p *Postgres) CriarDeEvento(ctx context.Context, eventoID string, t *titulo.Titulo) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO eventos_processados (evento_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventoID)
	if err != nil {
		return false, fmt.Errorf("registrar evento %s: %w", eventoID, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, sqlUpsert, argsTitulo(t)...); err != nil {
		return false, fmt.Errorf("criar título do evento %s: %w", eventoID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("confirmar evento %s: %w", eventoID, err)
	}
	return true, nil
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
	// Outbox: o evento é gravado junto com o pagamento. Ou os dois ficam, ou
	// nenhum fica; quem publica no RabbitMQ é o relay, depois do commit.
	if t.Status == titulo.StatusPago {
		payload, _ := json.Marshal(eventos.TituloPago{
			TituloID: t.ID, VendaID: t.VendaID, ClienteID: t.ClienteID, ValorCentavos: t.ValorCentavos, PagoEm: time.Now().UTC(),
		})
		if _, err := tx.Exec(ctx,
			`INSERT INTO outbox (tipo, payload) VALUES ($1, $2)`, eventos.ChaveTituloPago, payload); err != nil {
			return nil, fmt.Errorf("gravar outbox: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar pagamento: %w", err)
	}
	return t, nil
}
