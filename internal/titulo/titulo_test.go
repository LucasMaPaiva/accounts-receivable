package titulo

import (
	"errors"
	"testing"
	"time"
)

func data(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestNovo(t *testing.T) {
	casos := []struct {
		nome    string
		cliente string
		valor   int64
		erro    error
	}{
		{"válido", "c1", 10000, nil},
		{"sem cliente", "", 10000, ErrClienteObrigatorio},
		{"valor zero", "c1", 0, ErrValorInvalido},
		{"valor negativo", "c1", -5, ErrValorInvalido},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Novo("t1", c.cliente, "Mensalidade", c.valor, data("2026-12-10"))
			if !errors.Is(err, c.erro) {
				t.Fatalf("erro = %v, esperado %v", err, c.erro)
			}
		})
	}
}

func TestRegistrarPagamento(t *testing.T) {
	tit, _ := Novo("t1", "c1", "Mensalidade", 10000, data("2026-12-10"))

	if err := tit.RegistrarPagamento(4000); err != nil {
		t.Fatal(err)
	}
	if tit.Status != StatusParcial || tit.Saldo() != 6000 {
		t.Fatalf("depois do parcial: status %s, saldo %d", tit.Status, tit.Saldo())
	}

	if err := tit.RegistrarPagamento(7000); !errors.Is(err, ErrPagamentoExcede) {
		t.Fatalf("esperado ErrPagamentoExcede, veio %v", err)
	}

	if err := tit.RegistrarPagamento(6000); err != nil {
		t.Fatal(err)
	}
	if tit.Status != StatusPago || tit.Saldo() != 0 {
		t.Fatalf("depois da quitação: status %s, saldo %d", tit.Status, tit.Saldo())
	}

	if err := tit.RegistrarPagamento(1); !errors.Is(err, ErrJaPago) {
		t.Fatalf("esperado ErrJaPago, veio %v", err)
	}
}

func TestVencido(t *testing.T) {
	tit, _ := Novo("t1", "c1", "Mensalidade", 10000, data("2026-12-10"))

	if tit.Vencido(data("2026-12-10").Add(23 * time.Hour)) {
		t.Fatal("no próprio dia do vencimento ainda não está vencido")
	}
	if !tit.Vencido(data("2026-12-11")) {
		t.Fatal("no dia seguinte deveria estar vencido")
	}
	_ = tit.RegistrarPagamento(10000)
	if tit.Vencido(data("2027-01-01")) {
		t.Fatal("título pago nunca está vencido")
	}
}
