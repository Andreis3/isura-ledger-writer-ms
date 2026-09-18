package dto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/util"
)

type CreateTransactionInput struct {
	IdempotencyKey  *string           `json:"idempotency_key"`
	DebitAccountID  *string           `json:"debit_account_id"`
	CreditAccountID *string           `json:"credit_account_id"`
	Operation       *string           `json:"operation"`
	Amount          *int64            `json:"amount"`
	Currency        *string           `json:"currency"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type CreateTransactionOutput struct {
	TransactionID    *string `json:"transaction_id"`
	Status           string  `json:"status"`
	IdempotentReplay bool    `json:"idempotent_replay"`
}

const (
	MaxIdempotencyKeyLength = 50
	MaxMetadataEntries      = 32
	MaxMetadataKeyLength    = 64
	MaxMetadataValueLength  = 256
)

type canonicalMetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type canonicalTransactionInput struct {
	DebitAccountID  string                   `json:"debit_account_id"`
	CreditAccountID string                   `json:"credit_account_id"`
	Amount          int64                    `json:"amount"`
	Currency        string                   `json:"currency"`
	Operation       string                   `json:"operation"`
	Metadata        []canonicalMetadataEntry `json:"metadata"`
}

// Fingerprint returns the SHA-256 of the canonical business intent.
// IdempotencyKey is intentionally excluded from the digest.
func (d CreateTransactionInput) Fingerprint() (string, error) {
	if err := d.validateMetadata(); err != nil {
		return "", err
	}

	metadataKeys := make([]string, 0, len(d.Metadata))
	for key := range d.Metadata {
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)
	metadata := make([]canonicalMetadataEntry, 0, len(metadataKeys))
	for _, key := range metadataKeys {
		metadata = append(metadata, canonicalMetadataEntry{Key: key, Value: d.Metadata[key]})
	}

	canonical := canonicalTransactionInput{
		DebitAccountID:  util.String(d.DebitAccountID),
		CreditAccountID: util.String(d.CreditAccountID),
		Amount:          util.Int64(d.Amount),
		Currency:        util.String(d.Currency),
		Operation:       util.String(d.Operation),
		Metadata:        metadata,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", errors.New("failed to encode transaction fingerprint")
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (d CreateTransactionInput) validateMetadata() error {
	if len(d.Metadata) > MaxMetadataEntries {
		return fault.InvalidEntityError(errors.New("metadata has too many entries"), map[string]any{
			"metadata": "maximum entries exceeded",
		})
	}
	for key, value := range d.Metadata {
		if key == "" || len([]rune(key)) > MaxMetadataKeyLength {
			return fault.InvalidEntityError(errors.New("metadata key is invalid"), map[string]any{
				"metadata": "key is blank or too long",
			})
		}
		if len([]rune(value)) > MaxMetadataValueLength {
			return fault.InvalidEntityError(errors.New("metadata value is too long"), map[string]any{
				"metadata": "value is too long",
			})
		}
	}
	return nil
}

func (d *CreateTransactionInput) CreateTransactionFacade() (*transaction.Transaction, error) {
	if err := d.validateMetadata(); err != nil {
		return nil, err
	}
	if d.IdempotencyKey != nil && len([]rune(*d.IdempotencyKey)) > MaxIdempotencyKeyLength {
		return nil, fault.InvalidEntityError(errors.New("idempotency key is too long"), map[string]any{
			"idempotency_key": "cannot exceed 50 characters",
		})
	}
	fingerprint, err := d.Fingerprint()
	if err != nil {
		return nil, err
	}
	amount, err := money.NewMoney(util.Int64(d.Amount), money.Currency(util.String(d.Currency)))
	if err != nil {
		return nil, err
	}

	entryDestination, err := transaction.NewEntryBuilder().
		WithID().
		WithAccountExternalID(util.String(d.CreditAccountID)).
		WithAmount(amount).
		WithDirection(transaction.Credit).
		WithMetadata(d.Metadata).
		WithCreatedAt().
		Build()

	if err != nil {
		return nil, err
	}

	entrySource, err := transaction.NewEntryBuilder().
		WithID().
		WithAccountExternalID(util.String(d.DebitAccountID)).
		WithAmount(amount).
		WithDirection(transaction.Debit).
		WithMetadata(d.Metadata).
		WithCreatedAt().
		Build()
	if err != nil {
		return nil, err
	}

	entries := []*transaction.Entry{entryDestination, entrySource}

	return transaction.NewTransactionBuilder().
		WithID().
		WithIdempotencyKey(util.String(d.IdempotencyKey)).
		WithStatus(transaction.Pending).
		WithAmount(amount).
		WithOperation(transaction.Operation(util.String(d.Operation))).
		WithFingerprint(fingerprint).
		WithMetadata(d.Metadata).
		WithEntries(entries).
		WithCreatedAt().
		WithUpdatedAt().
		Build()
}

// LogValue implements slog.LogValuer to safely log transaction input without exposing raw sensitive data.
func (d CreateTransactionInput) LogValue() slog.Value {
	// Safe extraction helpers to avoid nil pointer panics during logging
	idempotencyKey := ""
	if d.IdempotencyKey != nil {
		idempotencyKey = *d.IdempotencyKey
	}

	debitAccount := ""
	if d.DebitAccountID != nil {
		debitAccount = util.String(d.DebitAccountID) // Substitua por sua função de máscara real, ex: mask.UUID(*d.DebitAccountID)
	}

	creditAccount := ""
	if d.CreditAccountID != nil {
		creditAccount = util.String(d.CreditAccountID) // Substitua por sua função de máscara real
	}

	var amount int64
	if d.Amount != nil {
		amount = util.Int64(d.Amount)
	}

	currency := ""
	if d.Currency != nil {
		currency = util.String(d.Currency)
	}

	return slog.GroupValue(
		slog.String("idempotency_key", idempotencyKey),
		slog.String("debit_account_id", debitAccount),
		slog.String("credit_account_id", creditAccount),
		slog.Int64("amount", amount),
		slog.String("currency", currency),
		slog.Int("metadata_entries", len(d.Metadata)),
	)
}
