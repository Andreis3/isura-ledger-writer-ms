package criteria

import (
	"strings"
)

// AccountCriteria define os filtros e opções de lock para consultas de contas
type AccountCriteria struct {
	ID                   *string
	AccountExternalID    *string
	TaxID                *string
	AccountNumber        *string
	Currency             *string
	Type                 *string
	HasForUpdate         bool // Bloqueio pessimista tradicional (aguarda a liberação do registo)
	HasForUpdateSkipLock bool // Bloqueio que ignora registos já bloqueados (uso restrito)
}

// GetAccountCriteria constrói dinamicamente a query SQL e os argumentos para a tabela accounts
func GetAccountCriteria(baseQuery string, params AccountCriteria) (string, []any) {
	// Pré-aloca o slice com a capacidade máxima estimada de argumentos (filtros + folga)
	args := make([]any, 0, 7)
	argCount := 1

	// Estima o tamanho aproximado da query no Builder para otimizar alocações de memória
	var sb strings.Builder
	sb.Grow(len(baseQuery) + 128)
	sb.WriteString(baseQuery)

	if params.ID != nil {
		sb.WriteString(" AND id = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.ID)
		argCount++
	}

	if params.AccountExternalID != nil {
		sb.WriteString(" AND account_external_id = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.AccountExternalID)
		argCount++
	}

	if params.TaxID != nil {
		sb.WriteString(" AND tax_id = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.TaxID)
		argCount++
	}

	if params.AccountNumber != nil {
		sb.WriteString(" AND account_number = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.AccountNumber)
		argCount++
	}

	if params.Currency != nil {
		sb.WriteString(" AND currency = $")
		sb.WriteString(argNumToString(argCount))
		args = append(args, *params.Currency)
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
