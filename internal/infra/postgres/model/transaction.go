package model

import (
	"encoding/json"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/jackc/pgx/v5/pgtype"
)

type Transaction struct {
	ID                 pgtype.Text
	IdempotencyKey     pgtype.Text
	RequestFingerprint pgtype.Text
	Status             pgtype.Text
	Operation          pgtype.Text
	Amount             pgtype.Int8
	Currency           pgtype.Text
	Metadata           []byte
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

func metadataBytes(metadata map[string]string) ([]byte, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	return json.Marshal(metadata)
}

func metadataMap(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var metadata map[string]string
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	return metadata, nil
}

func ToTransactionModel(domain *transaction.Transaction) (Transaction, error) {
	metadata, err := metadataBytes(domain.Metadata)
	if err != nil {
		return Transaction{}, fmt.Errorf("encode transaction metadata: %w", err)
	}
	return Transaction{
		ID: pgtype.Text{
			String: domain.ID.String(),
			Valid:  true,
		},
		IdempotencyKey: pgtype.Text{
			String: domain.IdempotencyKey,
			Valid:  true,
		},
		RequestFingerprint: pgtype.Text{
			String: domain.Fingerprint,
			Valid:  true,
		},
		Status: pgtype.Text{
			String: string(domain.Status),
			Valid:  true,
		},
		Operation: pgtype.Text{
			String: string(domain.Operation),
			Valid:  true,
		},
		Amount: pgtype.Int8{
			Int64: domain.Amount.Amount(),
			Valid: true,
		},
		Currency: pgtype.Text{
			String: string(domain.Amount.Currency()),
			Valid:  true,
		},
		Metadata: metadata,
		CreatedAt: pgtype.Timestamptz{
			Time:  domain.CreatedAt,
			Valid: true,
		},
		UpdatedAt: pgtype.Timestamptz{
			Time:  domain.UpdatedAt,
			Valid: true,
		},
	}, nil
}

func ToTransactionDomain(model Transaction, entries []*transaction.Entry) (*transaction.Transaction, error) {
	amount, err := money.NewMoney(model.Amount.Int64, money.Currency(model.Currency.String))
	if err != nil {
		return nil, err
	}

	id, err := entity.NewID(model.ID.String)
	if err != nil {
		return nil, err
	}
	metadata, err := metadataMap(model.Metadata)
	if err != nil {
		return nil, err
	}

	return transaction.NewTransactionBuilder().
		WithID(id.String()).
		WithIdempotencyKey(model.IdempotencyKey.String).
		WithFingerprint(model.RequestFingerprint.String).
		WithStatus(transaction.TransactionStatus(model.Status.String)).
		WithAmount(amount).
		WithOperation(transaction.Operation(model.Operation.String)).
		WithMetadata(metadata).
		WithCreatedAt(model.CreatedAt.Time).
		WithUpdatedAt(model.UpdatedAt.Time).
		WithEntries(entries).
		Build()

}
