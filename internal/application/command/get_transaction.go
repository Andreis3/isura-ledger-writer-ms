package command

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

type GetTransaction struct {
	repository transaction.Repository
}

func NewGetTransaction(repository transaction.Repository) *GetTransaction {
	return &GetTransaction{repository: repository}
}

func (c *GetTransaction) Execute(ctx context.Context, id string) (*transaction.Transaction, error) {
	result, err := c.repository.Find(ctx, transaction.TransactionCriteria{ID: &id, WithEntries: true})
	if errors.Is(err, transaction.ErrTransactionNotFound) {
		return nil, fault.ErrTransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
