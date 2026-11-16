package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LucasMaPaiva/accounts-receivable/internal/consulta"
)

// falsas guarda os argumentos recebidos para o teste conferir.
type falsas struct {
	hoje   time.Time
	limite int
}

func (f *falsas) Saldo(_ context.Context, clienteID string, hoje time.Time) (consulta.Saldo, error) {
	f.hoje = hoje
	return consulta.Saldo{ClienteID: clienteID, TitulosEmAberto: 2, EmAbertoCentavos: 12000, VencidoCentavos: 7000}, nil
}

func (f *falsas) Vencidos(_ context.Context, clienteID string, hoje time.Time, limite int) ([]consulta.Vencido, error) {
	f.hoje, f.limite = hoje, limite
	if clienteID == "sem-vencidos" {
		return nil, nil
	}
	return []consulta.Vencido{
		{TituloID: "t3", ClienteID: "c2", SaldoCentavos: 8000, Vencimento: "2026-10-17", DiasAtraso: 30},
		{TituloID: "t1", ClienteID: "c1", SaldoCentavos: 7000, Vencimento: "2026-11-01", DiasAtraso: 15},
	}, nil
}

func (f *falsas) Inadimplencia(_ context.Context, hoje time.Time, limite int) ([]consulta.Inadimplente, error) {
	f.hoje, f.limite = hoje, limite
	return []consulta.Inadimplente{{ClienteID: "c2", TitulosVencidos: 2, VencidoCentavos: 20000, MaiorAtrasoDias: 30}}, nil
}

// conectar liga um cliente MCP ao servidor em memória, sem rede nem stdio.
func conectar(t *testing.T, f *falsas) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	// 23h30 em Brasília do dia 16 já é dia 17 em UTC: "hoje" tem de ser 16.
	brasilia := time.FixedZone("BRT", -3*60*60)
	agora := func() time.Time { return time.Date(2026, 11, 16, 23, 30, 0, 0, brasilia) }

	transCliente, transServidor := mcp.NewInMemoryTransports()
	if _, err := Novo(f, agora).Connect(ctx, transServidor, nil); err != nil {
		t.Fatal(err)
	}
	sessao, err := mcp.NewClient(&mcp.Implementation{Name: "teste"}, nil).Connect(ctx, transCliente, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessao.Close() })
	return sessao
}

func chamar[T any](t *testing.T, s *mcp.ClientSession, nome string, args map[string]any) (T, *mcp.CallToolResult) {
	t.Helper()
	var saida T
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: nome, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", nome, err)
	}
	if !res.IsError {
		bruto, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(bruto, &saida); err != nil {
			t.Fatalf("%s: saída %s: %v", nome, bruto, err)
		}
	}
	return saida, res
}

func TestFerramentasSaoSomenteLeitura(t *testing.T) {
	s := conectar(t, &falsas{})
	lista, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista.Tools) != 3 {
		t.Fatalf("esperava 3 ferramentas, vieram %d", len(lista.Tools))
	}
	for _, f := range lista.Tools {
		if f.Annotations == nil || !f.Annotations.ReadOnlyHint {
			t.Errorf("%s deveria ser marcada como só leitura", f.Name)
		}
	}
}

func TestSaldoCliente(t *testing.T) {
	f := &falsas{}
	s := conectar(t, f)

	saldo, _ := chamar[consulta.Saldo](t, s, "saldo_cliente", map[string]any{"cliente_id": "c1"})
	if saldo.ClienteID != "c1" || saldo.VencidoCentavos != 7000 {
		t.Fatalf("saldo: %+v", saldo)
	}
	if got := f.hoje.Format(time.DateOnly); got != "2026-11-16" {
		t.Fatalf("hoje deveria seguir o fuso de agora: %s", got)
	}

	if _, res := chamar[consulta.Saldo](t, s, "saldo_cliente", map[string]any{"cliente_id": ""}); !res.IsError {
		t.Fatal("cliente vazio deveria voltar como erro da ferramenta")
	}
	// Sem o campo obrigatório, nem chega no handler: o schema de entrada barra.
	f.hoje = time.Time{}
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "saldo_cliente", Arguments: map[string]any{}})
	if (err == nil && !res.IsError) || !f.hoje.IsZero() {
		t.Fatal("chamada sem cliente_id deveria ser recusada antes do handler")
	}
}

func TestTitulosVencidos(t *testing.T) {
	f := &falsas{}
	s := conectar(t, f)

	saida, _ := chamar[SaidaVencidos](t, s, "titulos_vencidos", nil)
	if saida.Hoje != "2026-11-16" || saida.TotalCentavos != 15000 || len(saida.Titulos) != 2 || f.limite != 50 {
		t.Fatalf("vencidos: %+v limite=%d", saida, f.limite)
	}

	chamar[SaidaVencidos](t, s, "titulos_vencidos", map[string]any{"limite": 100000})
	if f.limite != 500 {
		t.Fatalf("limite deveria ter teto 500, veio %d", f.limite)
	}

	vazia, res := chamar[SaidaVencidos](t, s, "titulos_vencidos", map[string]any{"cliente_id": "sem-vencidos"})
	if res.IsError || vazia.Titulos == nil || len(vazia.Titulos) != 0 {
		t.Fatalf("lista vazia deveria ser [], não null: %+v", vazia)
	}
}

func TestInadimplencia(t *testing.T) {
	f := &falsas{}
	s := conectar(t, f)

	saida, _ := chamar[SaidaInadimplencia](t, s, "inadimplencia", map[string]any{"limite": 3})
	if len(saida.Clientes) != 1 || saida.Clientes[0].VencidoCentavos != 20000 || f.limite != 3 {
		t.Fatalf("inadimplência: %+v limite=%d", saida, f.limite)
	}
}
