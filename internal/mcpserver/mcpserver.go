// Package mcpserver expõe as consultas de contas a receber como ferramentas
// MCP, para um assistente (Claude, por exemplo) responder perguntas como
// "quem está devendo mais?" consultando os dados reais. Só leitura.
package mcpserver

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LucasMaPaiva/accounts-receivable/internal/consulta"
)

// Consultas é o que o servidor precisa; o teste usa uma implementação falsa.
type Consultas interface {
	Saldo(ctx context.Context, clienteID string, hoje time.Time) (consulta.Saldo, error)
	Vencidos(ctx context.Context, clienteID string, hoje time.Time, limite int) ([]consulta.Vencido, error)
	Inadimplencia(ctx context.Context, hoje time.Time, limite int) ([]consulta.Inadimplente, error)
}

// Entradas. Campo sem omitempty é obrigatório no schema gerado.
type (
	EntradaSaldo struct {
		ClienteID string `json:"cliente_id" jsonschema:"id do cliente"`
	}
	EntradaVencidos struct {
		ClienteID string `json:"cliente_id,omitempty" jsonschema:"id do cliente; vazio traz todos"`
		Limite    int    `json:"limite,omitempty" jsonschema:"máximo de títulos (padrão 50, máximo 500)"`
	}
	EntradaInadimplencia struct {
		Limite int `json:"limite,omitempty" jsonschema:"máximo de clientes (padrão 10, máximo 100)"`
	}
)

// Saídas: sempre objetos (o protocolo exige) e com a data usada como "hoje".
type (
	SaidaVencidos struct {
		Hoje          string             `json:"hoje"`
		TotalCentavos int64              `json:"total_centavos"`
		Titulos       []consulta.Vencido `json:"titulos"`
	}
	SaidaInadimplencia struct {
		Hoje     string                  `json:"hoje"`
		Clientes []consulta.Inadimplente `json:"clientes"`
	}
)

var ErrClienteObrigatorio = errors.New("cliente_id é obrigatório")

// Novo monta o servidor. agora vem de fora para o teste fixar a data; o dia
// é o do fuso de agora (Brasília no main).
func Novo(c Consultas, agora func() time.Time) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "contas-a-receber", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Consultas só leitura de contas a receber. Valores em centavos (15000 = R$ 150,00). " +
			"Datas no formato AAAA-MM-DD.",
	})
	somenteLeitura := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	hoje := func() time.Time {
		y, m, d := agora().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "saldo_cliente",
		Description: "Saldo em aberto de um cliente: quanto deve, quanto já venceu e o próximo vencimento.",
		Annotations: somenteLeitura,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaSaldo) (*mcp.CallToolResult, consulta.Saldo, error) {
		if in.ClienteID == "" {
			return nil, consulta.Saldo{}, ErrClienteObrigatorio
		}
		saldo, err := c.Saldo(ctx, in.ClienteID, hoje())
		return nil, saldo, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "titulos_vencidos",
		Description: "Títulos vencidos e não pagos, do mais antigo para o mais novo, com os dias de atraso.",
		Annotations: somenteLeitura,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaVencidos) (*mcp.CallToolResult, SaidaVencidos, error) {
		h := hoje()
		lista, err := c.Vencidos(ctx, in.ClienteID, h, limitar(in.Limite, 50, 500))
		if err != nil {
			return nil, SaidaVencidos{}, err
		}
		saida := SaidaVencidos{Hoje: h.Format(time.DateOnly), Titulos: lista}
		if saida.Titulos == nil {
			saida.Titulos = []consulta.Vencido{}
		}
		for _, v := range lista {
			saida.TotalCentavos += v.SaldoCentavos
		}
		return nil, saida, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "inadimplencia",
		Description: "Clientes com títulos vencidos, ordenados pelo valor vencido (maior primeiro).",
		Annotations: somenteLeitura,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntradaInadimplencia) (*mcp.CallToolResult, SaidaInadimplencia, error) {
		h := hoje()
		lista, err := c.Inadimplencia(ctx, h, limitar(in.Limite, 10, 100))
		if err != nil {
			return nil, SaidaInadimplencia{}, err
		}
		if lista == nil {
			lista = []consulta.Inadimplente{}
		}
		return nil, SaidaInadimplencia{Hoje: h.Format(time.DateOnly), Clientes: lista}, nil
	})

	return s
}

// limitar aplica o padrão quando o valor não veio e o teto para não trazer
// a tabela inteira para dentro do contexto do assistente.
func limitar(v, padrao, maximo int) int {
	switch {
	case v <= 0:
		return padrao
	case v > maximo:
		return maximo
	default:
		return v
	}
}
