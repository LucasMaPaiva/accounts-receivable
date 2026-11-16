package mensageria

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/LucasMaPaiva/accounts-receivable/internal/eventos"
	"github.com/LucasMaPaiva/accounts-receivable/internal/metricas"
)

// RelayOutbox publica os eventos pendentes da outbox a cada intervalo.
//
// Garantia "pelo menos uma vez": a linha só é marcada como publicada depois
// da confirmação do RabbitMQ (publisher confirm). Se o worker cair entre a
// confirmação e o UPDATE, o evento sai de novo; por isso o consumidor do outro
// lado também precisa ser idempotente (usa o message_id).
func RelayOutbox(ctx context.Context, pool *pgxpool.Pool, ch *amqp.Channel, intervalo time.Duration) error {
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("ativar publisher confirms: %w", err)
	}
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			n, err := publicarLote(ctx, pool, ch)
			if err != nil {
				slog.Error("relay da outbox", "erro", err)
				continue
			}
			if n > 0 {
				slog.Info("outbox publicada", "eventos", n)
			}
		}
	}
}

func publicarLote(ctx context.Context, pool *pgxpool.Pool, ch *amqp.Channel) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// SKIP LOCKED: com dois workers rodando, cada um pega linhas diferentes
	// em vez de um esperar o outro.
	rows, err := tx.Query(ctx, `
		SELECT id, tipo, payload FROM outbox
		WHERE publicado_em IS NULL
		ORDER BY id
		LIMIT 50
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type pendente struct {
		id      int64
		tipo    string
		payload []byte
	}
	var lote []pendente
	for rows.Next() {
		var p pendente
		if err := rows.Scan(&p.id, &p.tipo, &p.payload); err != nil {
			rows.Close()
			return 0, err
		}
		lote = append(lote, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	publicados := make([]int64, 0, len(lote))
	for _, p := range lote {
		confirmacao, err := ch.PublishWithDeferredConfirmWithContext(ctx, eventos.ExchangeContas, p.tipo, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				MessageId:    "outbox-" + strconv.FormatInt(p.id, 10),
				Timestamp:    time.Now(),
				Type:         p.tipo,
				Body:         p.payload,
			})
		if err != nil {
			break
		}
		ok, err := confirmacao.WaitContext(ctx)
		if err != nil || !ok {
			break
		}
		publicados = append(publicados, p.id)
	}
	if len(publicados) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE outbox SET publicado_em = now() WHERE id = ANY($1)`, publicados); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	metricas.OutboxPublicados.Add(float64(len(publicados)))
	return len(publicados), nil
}
