package repositorio

import (
	"context"
	"sort"
	"sync"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

// Memoria é segura para várias goroutines ao mesmo tempo: o servidor HTTP
// do Go atende cada requisição numa goroutine, todas no mesmo processo,
// então o mapa precisa de trava (no PHP-FPM cada requisição tem memória própria).
type Memoria struct {
	mu      sync.RWMutex
	titulos map[string]titulo.Titulo
	eventos map[string]bool
}

func NovaMemoria() *Memoria {
	return &Memoria{titulos: make(map[string]titulo.Titulo), eventos: make(map[string]bool)}
}

func (m *Memoria) CriarDeEvento(_ context.Context, eventoID string, t *titulo.Titulo) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventos[eventoID] {
		return false, nil
	}
	m.eventos[eventoID] = true
	m.titulos[t.ID] = *t
	return true, nil
}

func (m *Memoria) Salvar(_ context.Context, t *titulo.Titulo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.titulos[t.ID] = *t
	return nil
}

func (m *Memoria) Buscar(_ context.Context, id string) (*titulo.Titulo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.titulos[id]
	if !ok {
		return nil, ErrNaoEncontrado
	}
	return &t, nil
}

func (m *Memoria) Listar(_ context.Context, f Filtro) ([]titulo.Titulo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lista := []titulo.Titulo{}
	for _, t := range m.titulos {
		if f.ClienteID != "" && t.ClienteID != f.ClienteID {
			continue
		}
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if !f.VencidosEm.IsZero() && !t.Vencido(f.VencidosEm) {
			continue
		}
		lista = append(lista, t)
	}
	sort.Slice(lista, func(i, j int) bool {
		if !lista[i].Vencimento.Equal(lista[j].Vencimento) {
			return lista[i].Vencimento.Before(lista[j].Vencimento)
		}
		return lista[i].ID < lista[j].ID
	})
	if len(lista) > LimiteListagem {
		lista = lista[:LimiteListagem]
	}
	return lista, nil
}

// RegistrarPagamento lê, aplica a regra e grava sob a mesma trava: dois
// pagamentos simultâneos nunca enxergam o mesmo saldo.
func (m *Memoria) RegistrarPagamento(_ context.Context, id string, valorCentavos int64) (*titulo.Titulo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.titulos[id]
	if !ok {
		return nil, ErrNaoEncontrado
	}
	if err := t.RegistrarPagamento(valorCentavos); err != nil {
		return nil, err
	}
	m.titulos[id] = t
	return &t, nil
}
