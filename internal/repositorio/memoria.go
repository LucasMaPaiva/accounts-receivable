// Package repositorio guarda os títulos. Esta versão fica em memória;
// a próxima fase troca por PostgreSQL sem mexer no handler, porque o handler
// depende só da interface api.Repositorio.
package repositorio

import (
	"errors"
	"sync"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

var ErrNaoEncontrado = errors.New("título não encontrado")

// Memoria é segura para várias goroutines ao mesmo tempo: o servidor HTTP
// do Go atende cada requisição numa goroutine, todas no mesmo processo,
// então o mapa precisa de trava (no PHP-FPM cada requisição tem memória própria).
type Memoria struct {
	mu      sync.RWMutex
	titulos map[string]titulo.Titulo
}

func NovaMemoria() *Memoria {
	return &Memoria{titulos: make(map[string]titulo.Titulo)}
}

func (m *Memoria) Salvar(t *titulo.Titulo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.titulos[t.ID] = *t
	return nil
}

func (m *Memoria) Buscar(id string) (*titulo.Titulo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.titulos[id]
	if !ok {
		return nil, ErrNaoEncontrado
	}
	return &t, nil
}
