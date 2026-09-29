package criteria

import (
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

// GetTransactionCriteria traduz os critérios do domínio para filtros SQL parametrizados.
func GetTransactionCriteria(baseQuery string, params transaction.TransactionCriteria) (string, []any) {
	// Pré-aloca o slice com a capacidade máxima estimada de argumentos (filtros + folga)
	args := make([]any, 0, 6)
	argCount := 1

	// Estima o tamanho aproximado da query no Builder para evitar reallocations de memória
	var sb strings.Builder
	sb.Grow(len(baseQuery) + 128)
	sb.WriteString(baseQuery)

	if params.ID != nil {
		sb.WriteString(" AND id = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.ID)
		argCount++
	}

	if params.IdempotencyKey != nil {
		sb.WriteString(" AND idempotency_key = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.IdempotencyKey)
		argCount++
	}

	sb.WriteString(" LIMIT 1")
	return sb.String(), args
}
