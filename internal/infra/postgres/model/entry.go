package model

import (
	"encoding/json"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/jackc/pgx/v5/pgtype"
)

type Entry struct {
	ID             pgtype.Text
	AccountID      pgtype.Text
	TransactionID  pgtype.Text
	SequenceNumber pgtype.Int8
	RunningBalance pgtype.Int8
	Direction      pgtype.Text
	Amount         pgtype.Int8
	Currency       pgtype.Text
	Metadata       []byte
	CreatedAt      pgtype.Timestamptz
}

func entryMetadataBytes(metadata map[string]string) ([]byte, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	return json.Marshal(metadata)
}

func entryMetadataMap(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var metadata map[string]string
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode entry metadata: %w", err)
	}
	return metadata, nil
}

func ToEntryModel(domain *transaction.Entry) (Entry, error) {
	metadata, err := entryMetadataBytes(domain.Metadata)
	if err != nil {
		return Entry{}, fmt.Errorf("encode entry metadata: %w", err)
	}
	return Entry{
		ID:             pgtype.Text{String: domain.ID.String(), Valid: true},
		AccountID:      pgtype.Text{String: domain.AccountID, Valid: true},
		TransactionID:  pgtype.Text{String: domain.TransactionID, Valid: true},
		SequenceNumber: pgtype.Int8{Int64: domain.SequenceNumber, Valid: true},
		RunningBalance: pgtype.Int8{Int64: domain.RunningBalance, Valid: true},
		Direction:      pgtype.Text{String: string(domain.Direction), Valid: true},
		Amount:         pgtype.Int8{Int64: domain.Amount.Amount(), Valid: true},
		Currency:       pgtype.Text{String: string(domain.Amount.Currency()), Valid: true},
		Metadata:       metadata,
		CreatedAt:      pgtype.Timestamptz{Time: domain.CreatedAt, Valid: true},
	}, nil
}

func ToEntryDomain(model Entry) (*transaction.Entry, error) {
	amount, err := money.NewMoney(model.Amount.Int64, money.Currency(model.Currency.String))
	if err != nil {
		return nil, err
	}

	id, err := entity.NewID(model.ID.String)
	if err != nil {
		return nil, err
	}
	metadata, err := entryMetadataMap(model.Metadata)
	if err != nil {
		return nil, err
	}

	entry, err := transaction.NewEntryBuilder().
		WithID(id.String()).
		WithTransactionID(model.TransactionID.String).
		WithAccountExternalID(model.AccountID.String).
		WithDirection(transaction.Direction(model.Direction.String)).
		WithAmount(amount).
		WithMetadata(metadata).
		WithCreatedAt(model.CreatedAt.Time).
		Build()
	if err != nil {
		return nil, err
	}
	entry.AddAccountID(model.AccountID.String)
	entry.SetSequenceNumber(model.SequenceNumber.Int64)
	entry.SetRunningBalance(model.RunningBalance.Int64)
	return entry, nil
}
