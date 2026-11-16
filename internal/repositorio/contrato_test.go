package repositorio

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

// Repositorio é o contrato que as duas implementações cumprem. Os testes
// abaixo rodam iguais na Memoria e no Postgres (postgres_test.go).
type Repositorio interface {
	Salvar(ctx context.Context, t *titulo.Titulo) error
	Buscar(ctx context.Context, id string) (*titulo.Titulo, error)
	Listar(ctx context.Context, f Filtro) ([]titulo.Titulo, error)
	RegistrarPagamento(ctx context.Context, id string, valorCentavos int64) (*titulo.Titulo, error)
	CriarDeEvento(ctx context.Context, eventoID string, t *titulo.Titulo) (bool, error)
}

func data(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func novo(t *testing.T, repo Repositorio, id, cliente string, valor int64, venc string) {
	t.Helper()
	tit, err := titulo.Novo(id, cliente, "Teste", valor, data(venc))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Salvar(context.Background(), tit); err != nil {
		t.Fatal(err)
	}
}

func testarContrato(t *testing.T, repo Repositorio) {
	ctx := context.Background()

	t.Run("salvar e buscar", func(t *testing.T) {
		novo(t, repo, "a1", "c1", 10000, "2026-12-10")
		got, err := repo.Buscar(ctx, "a1")
		if err != nil {
			t.Fatal(err)
		}
		if got.ValorCentavos != 10000 || got.Status != titulo.StatusAberto || !got.Vencimento.Equal(data("2026-12-10")) {
			t.Fatalf("lido diferente do salvo: %+v", got)
		}
	})

	t.Run("não encontrado", func(t *testing.T) {
		if _, err := repo.Buscar(ctx, "nao-existe"); !errors.Is(err, ErrNaoEncontrado) {
			t.Fatalf("esperado ErrNaoEncontrado, veio %v", err)
		}
		if _, err := repo.RegistrarPagamento(ctx, "nao-existe", 100); !errors.Is(err, ErrNaoEncontrado) {
			t.Fatalf("esperado ErrNaoEncontrado, veio %v", err)
		}
	})

	t.Run("regra do domínio passa pelo repositório", func(t *testing.T) {
		novo(t, repo, "a2", "c1", 5000, "2026-12-10")
		if _, err := repo.RegistrarPagamento(ctx, "a2", 6000); !errors.Is(err, titulo.ErrPagamentoExcede) {
			t.Fatalf("esperado ErrPagamentoExcede, veio %v", err)
		}
		got, err := repo.RegistrarPagamento(ctx, "a2", 2000)
		if err != nil || got.Status != titulo.StatusParcial || got.Saldo() != 3000 {
			t.Fatalf("pagamento parcial: %+v, %v", got, err)
		}
	})

	t.Run("pagamentos simultâneos não estouram o saldo", func(t *testing.T) {
		novo(t, repo, "a3", "c2", 10000, "2026-12-10")
		var wg sync.WaitGroup
		var mu sync.Mutex
		aceitos, recusados := 0, 0
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.RegistrarPagamento(ctx, "a3", 2000)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					aceitos++
				case errors.Is(err, titulo.ErrPagamentoExcede), errors.Is(err, titulo.ErrJaPago):
					recusados++
				default:
					t.Errorf("erro inesperado: %v", err)
				}
			}()
		}
		wg.Wait()
		got, _ := repo.Buscar(ctx, "a3")
		if aceitos != 5 || recusados != 5 || got.Saldo() != 0 || got.Status != titulo.StatusPago {
			t.Fatalf("aceitos %d, recusados %d, saldo %d, status %s", aceitos, recusados, got.Saldo(), got.Status)
		}
	})

	t.Run("evento repetido não duplica título", func(t *testing.T) {
		t1, _ := titulo.Novo("e-1", "c3", "Venda", 1000, data("2026-12-10"))
		t2, _ := titulo.Novo("e-2", "c3", "Venda", 1000, data("2026-12-10"))
		if criado, err := repo.CriarDeEvento(ctx, "evento-x", t1); err != nil || !criado {
			t.Fatalf("primeira entrega: criado=%v err=%v", criado, err)
		}
		if criado, err := repo.CriarDeEvento(ctx, "evento-x", t2); err != nil || criado {
			t.Fatalf("segunda entrega: criado=%v err=%v", criado, err)
		}
		if _, err := repo.Buscar(ctx, "e-2"); !errors.Is(err, ErrNaoEncontrado) {
			t.Fatal("a segunda entrega não deveria criar título")
		}
	})

	t.Run("listar com filtros", func(t *testing.T) {
		novo(t, repo, "l1", "c9", 1000, "2026-11-01")
		novo(t, repo, "l2", "c9", 1000, "2026-12-01")
		novo(t, repo, "l3", "c9", 1000, "2026-10-01")
		if _, err := repo.RegistrarPagamento(ctx, "l3", 1000); err != nil {
			t.Fatal(err)
		}

		todos, _ := repo.Listar(ctx, Filtro{ClienteID: "c9"})
		if ids(todos) != "l3,l1,l2" {
			t.Fatalf("ordem por vencimento: %s", ids(todos))
		}
		pagos, _ := repo.Listar(ctx, Filtro{ClienteID: "c9", Status: titulo.StatusPago})
		if ids(pagos) != "l3" {
			t.Fatalf("status pago: %s", ids(pagos))
		}
		vencidos, _ := repo.Listar(ctx, Filtro{ClienteID: "c9", VencidosEm: data("2026-11-16")})
		if ids(vencidos) != "l1" {
			t.Fatalf("vencidos em 16/11 (l3 está pago, l2 vence depois): %s", ids(vencidos))
		}
	})
}

func ids(ts []titulo.Titulo) string {
	s := ""
	for i, t := range ts {
		if i > 0 {
			s += ","
		}
		s += t.ID
	}
	return s
}

func TestMemoria(t *testing.T) {
	testarContrato(t, NovaMemoria())
}
