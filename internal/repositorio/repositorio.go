// Package repositorio guarda os títulos. Há duas implementações com o mesmo
// contrato: Memoria (testes e desenvolvimento) e Postgres (produção).
package repositorio

import (
	"errors"
	"time"

	"github.com/LucasMaPaiva/accounts-receivable/internal/titulo"
)

var ErrNaoEncontrado = errors.New("título não encontrado")

// Filtro da listagem. Campo vazio ou zero = sem filtro.
type Filtro struct {
	ClienteID string
	Status    titulo.Status
	// VencidosEm lista só os títulos com saldo e vencimento anterior a esta data.
	VencidosEm time.Time
}

// LimiteListagem evita devolver a tabela inteira numa requisição.
const LimiteListagem = 500
