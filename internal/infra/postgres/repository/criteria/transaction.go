package criteria

import "strings"

// TransactionCriteria define os filtros e opções de lock para consultas de transações
type TransactionCriteria struct {
	ID                   *string
	IdempotencyKey       *string
	Status               *string
	AccountID            *string
	Type                 *string
	HasForUpdate         bool // Bloqueio pessimista tradicional (aguarda a liberação do registo)
	HasForUpdateSkipLock bool // Bloqueio que ignora registos já bloqueados (usado apenas em filas específicas)
	WithEntries          bool
}

// GetTransactionCriteria constrói dinamicamente a query SQL e os respetivos argumentos com base nos critérios
func GetTransactionCriteria(baseQuery string, params TransactionCriteria) (string, []any) {
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

	if params.Status != nil {
		sb.WriteString(" AND status = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.Status)
		argCount++
	}

	if params.AccountID != nil {
		sb.WriteString(" AND account_id = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.AccountID)
		argCount++
	}

	if params.Type != nil {
		sb.WriteString(" AND type = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.Type)
		argCount++
	}

	// Adiciona a diretiva correta de bloqueio pessimista ao final da query
	if params.HasForUpdate {
		sb.WriteString(" FOR UPDATE")
	} else if params.HasForUpdateSkipLock {
		sb.WriteString(" FOR UPDATE SKIP LOCKED")
	}

	sb.WriteString(" LIMIT 1")

	return sb.String(), args
}
