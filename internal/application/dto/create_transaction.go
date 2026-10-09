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
	Entries         []EntryInput      `json:"entries,omitempty"`
	DebitAccountID  *string           `json:"debit_account_id"`
	CreditAccountID *string           `json:"credit_account_id"`
	Operation       *string           `json:"operation"`
	Amount          *int64            `json:"amount"`
	Currency        *string           `json:"currency"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// EntryInput is one ordered accounting posting requested by a client.
type EntryInput struct {
	AccountID string            `json:"account_id"`
	Direction string            `json:"direction"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Metadata  map[string]string `json:"metadata,omitempty"`
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

type canonicalEntryInput struct {
	AccountID string                   `json:"account_id"`
	Direction string                   `json:"direction"`
	Amount    int64                    `json:"amount"`
	Currency  string                   `json:"currency"`
	Metadata  []canonicalMetadataEntry `json:"metadata"`
}

type canonicalEntriesTransactionInput struct {
	Amount    int64                    `json:"amount"`
	Entries   []canonicalEntryInput    `json:"entries"`
	Operation string                   `json:"operation"`
	Metadata  []canonicalMetadataEntry `json:"metadata"`
}

// Fingerprint returns the SHA-256 of the canonical business intent.
// IdempotencyKey is intentionally excluded from the digest.
func (d CreateTransactionInput) Fingerprint() (string, error) {
	if err := d.validateMetadata(); err != nil {
		return "", err
	}
	if err := d.validateFormat(); err != nil {
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

	requestedEntries := d.normalizedEntries()
	entries := make([]canonicalEntryInput, 0, len(requestedEntries))
	for _, entry := range requestedEntries {
		entryMetadataValues := entry.Metadata
		if entryMetadataValues == nil {
			entryMetadataValues = d.Metadata
		}
		entryMetadata, err := canonicalMetadata(entryMetadataValues)
		if err != nil {
			return "", err
		}
		entries = append(entries, canonicalEntryInput{
			AccountID: entry.AccountID,
			Direction: entry.Direction,
			Amount:    entry.Amount,
			Currency:  entry.Currency,
			Metadata:  entryMetadata,
		})
	}
	canonical := canonicalEntriesTransactionInput{
		Amount: util.Int64(d.Amount), Entries: entries,
		Operation: util.String(d.Operation), Metadata: metadata,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", errors.New("failed to encode transaction fingerprint")
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (d CreateTransactionInput) validateFormat() error {
	if d.Entries == nil {
		return nil
	}
	if d.DebitAccountID != nil || d.CreditAccountID != nil || d.Currency != nil {
		return fault.InvalidEntityError(errors.New("entries cannot be combined with legacy transaction fields"), map[string]any{
			"entries": "cannot be combined with legacy fields",
		})
	}
	if d.Amount == nil || *d.Amount <= 0 {
		return fault.InvalidEntityError(errors.New("entries request must declare a positive transaction amount"), map[string]any{
			"amount": "must be a positive total for entries",
		})
	}
	if len(d.Entries) == 0 {
		return fault.InvalidEntityError(errors.New("entries must not be empty"), map[string]any{
			"entries": "must contain at least two entries",
		})
	}
	return nil
}

func canonicalMetadata(values map[string]string) ([]canonicalMetadataEntry, error) {
	if err := (CreateTransactionInput{Metadata: values}).validateMetadata(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]canonicalMetadataEntry, 0, len(keys))
	for _, key := range keys {
		result = append(result, canonicalMetadataEntry{Key: key, Value: values[key]})
	}
	return result, nil
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
	if err := d.validateFormat(); err != nil {
		return nil, err
	}
	fingerprint, err := d.Fingerprint()
	if err != nil {
		return nil, err
	}
	requestedEntries := d.normalizedEntries()
	entries := make([]*transaction.Entry, 0, len(requestedEntries))
	for _, requested := range requestedEntries {
		amount, err := money.NewMoney(requested.Amount, money.Currency(requested.Currency))
		if err != nil {
			return nil, err
		}
		metadata := requested.Metadata
		if metadata == nil {
			metadata = d.Metadata
		}
		entry, err := transaction.NewEntryBuilder().WithID().
			WithAccountExternalID(requested.AccountID).
			WithAmount(amount).
			WithDirection(transaction.Direction(requested.Direction)).
			WithMetadata(metadata).
			WithCreatedAt().Build()
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	amount, err := money.NewMoney(util.Int64(d.Amount), entries[0].Amount.Currency())
	if err != nil {
		return nil, err
	}

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

func (d CreateTransactionInput) normalizedEntries() []EntryInput {
	if d.Entries != nil {
		return append([]EntryInput(nil), d.Entries...)
	}
	return []EntryInput{
		{AccountID: util.String(d.CreditAccountID), Direction: transaction.Credit.String(), Amount: util.Int64(d.Amount), Currency: util.String(d.Currency), Metadata: d.Metadata},
		{AccountID: util.String(d.DebitAccountID), Direction: transaction.Debit.String(), Amount: util.Int64(d.Amount), Currency: util.String(d.Currency), Metadata: d.Metadata},
	}
}

// LogValue deliberately excludes raw idempotency keys, full account identifiers,
// entry payloads and metadata values from structured application logs.
func (d CreateTransactionInput) LogValue() slog.Value {
	idempotencyKey := ""
	if d.IdempotencyKey != nil && *d.IdempotencyKey != "" {
		idempotencyKey = "[REDACTED]"
	}

	debitAccount := ""
	if d.DebitAccountID != nil {
		debitAccount = maskAccountIDForLog(*d.DebitAccountID)
	}

	creditAccount := ""
	if d.CreditAccountID != nil {
		creditAccount = maskAccountIDForLog(*d.CreditAccountID)
	}

	var amount int64
	if d.Amount != nil {
		amount = *d.Amount
	}

	currency := ""
	if d.Currency != nil {
		currency = *d.Currency
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

// maskAccountIDForLog leaves only a short suffix for debugging correlation.
// Short identifiers are completely redacted instead of partially exposed.
func maskAccountIDForLog(value string) string {
	if value == "" {
		return ""
	}
	chars := []rune(value)
	if len(chars) <= 4 {
		return "[REDACTED]"
	}
	return "****" + string(chars[len(chars)-4:])
}
