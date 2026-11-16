// Package eventos define as mensagens trocadas com o monólito e o
// processamento delas, sem depender do RabbitMQ (fácil de testar).
package eventos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

// Nomes da topologia no RabbitMQ.
const (
	ExchangeVendas     = "vendas"      // o monólito publica aqui
	ExchangeContas     = "contas"      // este serviço publica aqui
	ExchangeErro       = "vendas.erro" // dead letter
	ChaveVendaFaturada = "venda.faturada"
	ChaveTituloPago    = "titulo.pago"
	FilaVendaFaturada  = "contas.venda_faturada"
	FilaErro           = "contas.venda_faturada.erro"
	// Fila que o monólito consome (exemplo da fase 4); declarada aqui para
	// a mensagem não se perder enquanto o lado PHP não existe.
	FilaMonolitoTituloPago = "monolito.titulo_pago"
)

// VendaFaturada chega do monólito quando uma venda é faturada.
type VendaFaturada struct {
	EventoID      string `json:"evento_id"`
	VendaID       string `json:"venda_id"`
	ClienteID     string `json:"cliente_id"`
	Descricao     string `json:"descricao"`
	ValorCentavos int64  `json:"valor_centavos"`
	Vencimento    string `json:"vencimento"` // AAAA-MM-DD
}

// TituloPago sai deste serviço quando um título é quitado.
type TituloPago struct {
	TituloID      string    `json:"titulo_id"`
	VendaID       string    `json:"venda_id,omitempty"`
	ClienteID     string    `json:"cliente_id"`
	ValorCentavos int64     `json:"valor_centavos"`
	PagoEm        time.Time `json:"pago_em"`
}

// ErrMensagemInvalida marca erro permanente: reprocessar não adianta, a
// mensagem vai para a fila de erro. Qualquer outro erro é tratado como
// temporário (banco fora do ar, por exemplo) e a mensagem volta para a fila.
var ErrMensagemInvalida = errors.New("mensagem inválida")

// Criador grava o título e marca o evento como processado na mesma
// transação. Devolve false se o evento já tinha sido processado.
type Criador interface {
	CriarDeEvento(ctx context.Context, eventoID string, t *titulo.Titulo) (bool, error)
}

func ProcessarVendaFaturada(ctx context.Context, repo Criador, corpo []byte, novoID func() string) (bool, error) {
	var ev VendaFaturada
	if err := json.Unmarshal(corpo, &ev); err != nil {
		return false, fmt.Errorf("%w: JSON: %v", ErrMensagemInvalida, err)
	}
	if ev.EventoID == "" || ev.VendaID == "" {
		return false, fmt.Errorf("%w: evento_id e venda_id são obrigatórios", ErrMensagemInvalida)
	}
	venc, err := time.Parse("2006-01-02", ev.Vencimento)
	if err != nil {
		return false, fmt.Errorf("%w: vencimento: %v", ErrMensagemInvalida, err)
	}
	descricao := ev.Descricao
	if descricao == "" {
		descricao = "Venda " + ev.VendaID
	}
	t, err := titulo.Novo(novoID(), ev.ClienteID, descricao, ev.ValorCentavos, venc)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrMensagemInvalida, err)
	}
	t.VendaID = ev.VendaID
	return repo.CriarDeEvento(ctx, ev.EventoID, t)
}
