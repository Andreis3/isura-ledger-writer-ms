package transaction

import (
	"context"
)

type Repository interface {
	Save(ctx context.Context, transaction *Transaction) error
	Find(ctx context.Context, params TransactionCriteria) (*Transaction, error)
	ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error)
}
