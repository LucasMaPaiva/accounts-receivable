// Package mensageria liga o serviço ao RabbitMQ: declara a topologia,
// consome venda.faturada e publica os eventos da outbox.
package mensageria

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/LucasMaPaiva/accounts-receivable/internal/eventos"
)

// Conectar tenta por até 30 s: no docker-compose o RabbitMQ pode ainda
// estar subindo quando o worker começa.
func Conectar(ctx context.Context, url string) (*amqp.Connection, error) {
	var ultimo error
	for range 15 {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		ultimo = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("conectar no RabbitMQ: %w", ultimo)
}

// DeclararTopologia é idempotente: pode rodar a cada inicialização.
//
//	vendas (topic) --venda.faturada--> contas.venda_faturada --(rejeitada)--> vendas.erro --> contas.venda_faturada.erro
//	contas (topic) --titulo.pago-----> monolito.titulo_pago
func DeclararTopologia(ch *amqp.Channel) error {
	for _, ex := range []struct{ nome, tipo string }{
		{eventos.ExchangeVendas, "topic"},
		{eventos.ExchangeContas, "topic"},
		{eventos.ExchangeErro, "fanout"},
	} {
		if err := ch.ExchangeDeclare(ex.nome, ex.tipo, true, false, false, false, nil); err != nil {
			return fmt.Errorf("exchange %s: %w", ex.nome, err)
		}
	}
	filas := []struct {
		nome, exchange, chave string
		args                  amqp.Table
	}{
		{eventos.FilaErro, eventos.ExchangeErro, "", nil},
		{eventos.FilaVendaFaturada, eventos.ExchangeVendas, eventos.ChaveVendaFaturada,
			// quorum: replicável; x-delivery-limit: depois de 10 entregas sem sucesso
			// (erro temporário que nunca passa), a mensagem vai para a fila de erro.
			amqp.Table{"x-dead-letter-exchange": eventos.ExchangeErro, "x-queue-type": "quorum", "x-delivery-limit": 10}},
		{eventos.FilaMonolitoTituloPago, eventos.ExchangeContas, eventos.ChaveTituloPago, nil},
	}
	for _, f := range filas {
		if _, err := ch.QueueDeclare(f.nome, true, false, false, false, f.args); err != nil {
			return fmt.Errorf("fila %s: %w", f.nome, err)
		}
		if err := ch.QueueBind(f.nome, f.chave, f.exchange, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", f.nome, err)
		}
	}
	return nil
}

// Consumir processa venda.faturada até o contexto ser cancelado.
//
//   - sucesso ou evento repetido: Ack (sai da fila)
//   - mensagem inválida: Nack sem requeue (vai para a fila de erro)
//   - erro temporário (banco fora do ar): espera e Nack com requeue (tenta de novo)
func Consumir(ctx context.Context, ch *amqp.Channel, repo eventos.Criador, novoID func() string) error {
	// No máximo 10 mensagens em processamento por vez neste worker.
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	msgs, err := ch.Consume(eventos.FilaVendaFaturada, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumir %s: %w", eventos.FilaVendaFaturada, err)
	}
	slog.Info("consumindo", "fila", eventos.FilaVendaFaturada)
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("canal do RabbitMQ fechado")
			}
			tratar(ctx, msg, repo, novoID)
		}
	}
}

func tratar(ctx context.Context, msg amqp.Delivery, repo eventos.Criador, novoID func() string) {
	criado, err := eventos.ProcessarVendaFaturada(ctx, repo, msg.Body, novoID)
	switch {
	case err == nil:
		slog.Info("venda.faturada processada", "message_id", msg.MessageId, "titulo_criado", criado)
		_ = msg.Ack(false)
	case errors.Is(err, eventos.ErrMensagemInvalida):
		slog.Warn("mensagem enviada para a fila de erro", "message_id", msg.MessageId, "erro", err)
		_ = msg.Nack(false, false)
	default:
		slog.Error("erro temporário, mensagem volta para a fila", "message_id", msg.MessageId, "erro", err)
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
		_ = msg.Nack(false, true)
	}
}
