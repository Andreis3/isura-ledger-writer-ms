package model

import (
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/jackc/pgx/v5/pgtype"
)

type Movement struct {
	ID            pgtype.Text
	TransactionID pgtype.Text
	Type          pgtype.Text
	Description   pgtype.Text
}

func ToMovementModel(domain *transaction.Movement, transactionID string) (Movement, error) {
	if domain == nil {
		return Movement{}, fmt.Errorf("movement cannot be nil")
	}
	return Movement{
		ID:            pgtype.Text{String: domain.ID.String(), Valid: true},
		TransactionID: pgtype.Text{String: transactionID, Valid: true},
		Type:          pgtype.Text{String: domain.Type, Valid: true},
		Description:   pgtype.Text{String: domain.Description, Valid: true},
	}, nil
}

func ToMovementDomain(persisted Movement, entries []*transaction.Entry) (*transaction.Movement, error) {
	return transaction.NewMovementBuilder().
		WithID(persisted.ID.String).
		WithType(persisted.Type.String).
		WithDescription(persisted.Description.String).
		WithEntries(entries).
		Build()
}
