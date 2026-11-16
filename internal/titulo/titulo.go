// Package titulo guarda as regras de negócio de um título a receber.
// Não conhece HTTP, banco nem fila: só o domínio.
package titulo

import (
	"errors"
	"time"
)

type Status string

const (
	StatusAberto  Status = "aberto"
	StatusParcial Status = "parcial"
	StatusPago    Status = "pago"
)

// Erros de domínio. Em Go o erro é um valor de retorno, não uma exceção:
// quem chama decide o que fazer com ele (errors.Is compara).
var (
	ErrValorInvalido      = errors.New("valor deve ser maior que zero")
	ErrClienteObrigatorio = errors.New("cliente é obrigatório")
	ErrJaPago             = errors.New("título já está pago")
	ErrPagamentoExcede    = errors.New("pagamento maior que o saldo")
)

// Titulo usa centavos (int64) para dinheiro: float perde precisão.
type Titulo struct {
	ID            string    `json:"id"`
	ClienteID     string    `json:"cliente_id"`
	Descricao     string    `json:"descricao"`
	ValorCentavos int64     `json:"valor_centavos"`
	PagoCentavos  int64     `json:"pago_centavos"`
	Vencimento    time.Time `json:"vencimento"`
	Status        Status    `json:"status"`
	// VendaID liga o título à venda do monólito que o originou (vazio se o
	// título foi criado direto pela API).
	VendaID string `json:"venda_id,omitempty"`
}

func Novo(id, clienteID, descricao string, valorCentavos int64, vencimento time.Time) (*Titulo, error) {
	if clienteID == "" {
		return nil, ErrClienteObrigatorio
	}
	if valorCentavos <= 0 {
		return nil, ErrValorInvalido
	}
	return &Titulo{
		ID:            id,
		ClienteID:     clienteID,
		Descricao:     descricao,
		ValorCentavos: valorCentavos,
		Vencimento:    vencimento,
		Status:        StatusAberto,
	}, nil
}

func (t *Titulo) Saldo() int64 {
	return t.ValorCentavos - t.PagoCentavos
}

// RegistrarPagamento aceita pagamento parcial ou total.
func (t *Titulo) RegistrarPagamento(valorCentavos int64) error {
	if valorCentavos <= 0 {
		return ErrValorInvalido
	}
	if t.Status == StatusPago {
		return ErrJaPago
	}
	if valorCentavos > t.Saldo() {
		return ErrPagamentoExcede
	}
	t.PagoCentavos += valorCentavos
	if t.Saldo() == 0 {
		t.Status = StatusPago
	} else {
		t.Status = StatusParcial
	}
	return nil
}

// Vencido compara só a data: o título vence no fim do dia do vencimento.
func (t *Titulo) Vencido(hoje time.Time) bool {
	if t.Saldo() == 0 {
		return false
	}
	y1, m1, d1 := hoje.Date()
	y2, m2, d2 := t.Vencimento.Date()
	return time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC).After(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC))
}
