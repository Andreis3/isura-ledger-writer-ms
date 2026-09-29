package transaction

// Criteria define os filtros funcionais para localizar uma transação.
type TransactionCriteria struct {
	ID             *string
	IdempotencyKey *string
	WithEntries    bool
}
