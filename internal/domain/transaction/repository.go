package transaction

import (
	"context"
)

type Repository interface {
	// Save persists a transaction whose entries already have ledger positions,
	// sequence numbers, and running balances assigned by the application flow.
	Save(ctx context.Context, transaction *Transaction) error
	Find(ctx context.Context, params TransactionCriteria) (*Transaction, error)
	FindLatestLedgerState(ctx context.Context, accountID string) (sequenceNumber, runningBalance int64, err error)
	ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error)
}
