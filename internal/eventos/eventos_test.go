package eventos_test

import (
	"context"
	"errors"
	"testing"

	"github.com/LucasMaPaiva/accounts-receivable/internal/eventos"
	"github.com/LucasMaPaiva/accounts-receivable/internal/repositorio"
)

func TestProcessarVendaFaturada(t *testing.T) {
	ctx := context.Background()
	repo := repositorio.NovaMemoria()
	n := 0
	novoID := func() string { n++; return "t" + string(rune('0'+n)) }

	valida := []byte(`{"evento_id":"e1","venda_id":"v1","cliente_id":"c1","valor_centavos":15000,"vencimento":"2026-12-10"}`)

	criado, err := eventos.ProcessarVendaFaturada(ctx, repo, valida, novoID)
	if err != nil || !criado {
		t.Fatalf("primeira entrega: criado=%v err=%v", criado, err)
	}
	tit, _ := repo.Buscar(ctx, "t1")
	if tit.ValorCentavos != 15000 || tit.Descricao != "Venda v1" || tit.VendaID != "v1" {
		t.Fatalf("título criado errado: %+v", tit)
	}

	criado, err = eventos.ProcessarVendaFaturada(ctx, repo, valida, novoID)
	if err != nil || criado {
		t.Fatalf("entrega repetida deveria ser ignorada: criado=%v err=%v", criado, err)
	}
	if lista, _ := repo.Listar(ctx, repositorio.Filtro{}); len(lista) != 1 {
		t.Fatalf("entrega repetida duplicou o título: %d títulos", len(lista))
	}

	invalidas := map[string]string{
		"JSON quebrado":  `{`,
		"sem evento_id":  `{"venda_id":"v2","cliente_id":"c1","valor_centavos":1,"vencimento":"2026-12-10"}`,
		"data inválida":  `{"evento_id":"e2","venda_id":"v2","cliente_id":"c1","valor_centavos":1,"vencimento":"10/12/2026"}`,
		"valor negativo": `{"evento_id":"e3","venda_id":"v3","cliente_id":"c1","valor_centavos":-1,"vencimento":"2026-12-10"}`,
	}
	for nome, corpo := range invalidas {
		t.Run(nome, func(t *testing.T) {
			if _, err := eventos.ProcessarVendaFaturada(ctx, repo, []byte(corpo), novoID); !errors.Is(err, eventos.ErrMensagemInvalida) {
				t.Fatalf("esperado ErrMensagemInvalida, veio %v", err)
			}
		})
	}
}
