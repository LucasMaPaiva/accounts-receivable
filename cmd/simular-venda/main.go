// simular-venda publica um venda.faturada como o monólito faria.
// Uso: docker compose run --rm simular-venda -cliente c1 -valor 15000
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/LucasMaPaiva/accounts-receivable/internal/eventos"
)

func main() {
	ev := eventos.VendaFaturada{}
	flag.StringVar(&ev.EventoID, "evento", aleatorio(), "id do evento (repita para testar a idempotência)")
	flag.StringVar(&ev.VendaID, "venda", "v-"+aleatorio()[:6], "id da venda no monólito")
	flag.StringVar(&ev.ClienteID, "cliente", "c1", "id do cliente")
	flag.Int64Var(&ev.ValorCentavos, "valor", 15000, "valor em centavos")
	flag.StringVar(&ev.Vencimento, "vencimento", time.Now().AddDate(0, 0, 30).Format("2006-01-02"), "AAAA-MM-DD")
	bruto := flag.String("bruto", "", "corpo cru da mensagem (para testar a fila de erro)")
	flag.Parse()

	corpo := []byte(*bruto)
	if *bruto == "" {
		corpo, _ = json.Marshal(ev)
	}

	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	err = ch.PublishWithContext(context.Background(), eventos.ExchangeVendas, eventos.ChaveVendaFaturada, false, false,
		amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: ev.EventoID, Body: corpo})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("publicado: %s\n", corpo)
}

func aleatorio() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
