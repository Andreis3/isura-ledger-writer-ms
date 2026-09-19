package transaction

import (
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/shared"
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

var (
	ErrInvalidDirection    = errors.New("invalid direction")
	ErrNegativeAmountValue = errors.New("amount cannot be negative")
	ErrAmountEqualZero     = errors.New("amount cannot be zero")
)

type Direction string

const (
	Credit Direction = "CREDIT"
	Debit  Direction = "DEBIT"
)

func (d Direction) IsValid() bool {
	switch d {
	case Credit, Debit:
		return true
	}
	return false
}

type EntryBuilder struct {
	id                entity.ID
	accountExternalID string
	transactionID     string
	direction         Direction
	amount            money.Money
	createdAt         time.Time
	metadata          map[string]string
	eval              validator.Evaluator
}

func NewEntryBuilder() *EntryBuilder {
	return &EntryBuilder{}
}

type Entry struct {
	ID                entity.ID
	AccountID         string
	AccountExternalID string
	TransactionID     string
	SequenceNumber    int64
	RunningBalance    int64
	Direction         Direction
	Amount            money.Money
	CreatedAt         time.Time
	Metadata          map[string]string
}

func (b *EntryBuilder) WithID(id ...string) *EntryBuilder {
	if len(id) > 0 {
		b.eval.CheckField(validator.NotBlank(id[0]), "id", "cannot be blank")
		b.eval.CheckField(validator.MatchesUUIDv7(id[0]), "id", "is not uuid")
		uuidV7, err := entity.NewID(id[0])
		if err != nil {
			b.eval.AddFieldError("id", err.Error())
		}

		b.id = uuidV7
		return b
	}

	uuidv7, err := entity.NewIDV7()
	if err != nil {
		b.eval.AddFieldError("id", err.Error())
	}
	b.id = uuidv7
	return b
}

func (b *EntryBuilder) WithTransactionID(transactionID string) *EntryBuilder {
	b.eval.CheckField(validator.NotBlank(transactionID), "transaction_id", "cannot be blank")
	b.eval.CheckField(validator.MatchesUUIDv7(transactionID), "transaction_id", "is not uuid")
	b.transactionID = transactionID
	return b
}

func (b *EntryBuilder) WithAccountExternalID(accountExternalID string) *EntryBuilder {
	b.eval.CheckField(validator.NotBlank(accountExternalID), "account_external_id", "cannot be blank")
	b.eval.CheckField(validator.MatchesUUID(accountExternalID), "account_external_id", "is not uuid")
	b.accountExternalID = accountExternalID
	return b
}

func (b *EntryBuilder) WithDirection(direction Direction) *EntryBuilder {
	d := Direction(direction)
	b.eval.CheckField(d.IsValid(), "direction", "invalid direction")
	b.direction = d
	return b
}

func (b *EntryBuilder) WithAmount(amount money.Money) *EntryBuilder {
	b.eval.CheckField(!amount.IsZero(), "amount", "cannot be zero")
	b.eval.CheckField(!amount.IsNegative(), "amount", "cannot be negative")
	b.eval.CheckField(amount.Currency().IsValid(), "amount", "invalid currency")
	b.amount = amount
	return b
}

func (b *EntryBuilder) WithCreatedAt(createdAt ...time.Time) *EntryBuilder {
	if len(createdAt) > 0 {
		if !createdAt[0].IsZero() && createdAt[0].After(time.Now()) {
			b.eval.CheckField(false, "updated_at", "cannot be in the future")
		}
		b.createdAt = createdAt[0]
		return b
	}

	b.createdAt = time.Now()
	return b
}

// WithMetadata stores a copy of the authorized metadata on the historical entry.
func (b *EntryBuilder) WithMetadata(metadata map[string]string) *EntryBuilder {
	if metadata == nil {
		return b
	}
	b.metadata = cloneMetadata(metadata)
	return b
}

func (b *EntryBuilder) Build() (*Entry, error) {
	if len(b.eval) > 0 {
		return nil, fault.InvalidEntityError(errors.New("invalid entry entity"), b.eval)
	}

	now := time.Now()

	return &Entry{
		ID:                b.id,
		TransactionID:     b.transactionID,
		AccountExternalID: b.accountExternalID,
		Direction:         b.direction,
		Amount:            b.amount,
		CreatedAt:         shared.CoalesceTime(b.createdAt, now),
		Metadata:          cloneMetadata(b.metadata),
	}, nil
}

func (e *Entry) AddAccountID(acountID string) {
	e.AccountID = acountID
}

// SetSequenceNumber records the immutable sequence assigned by the ledger for this account.
func (e *Entry) SetSequenceNumber(sequence int64) {
	e.SequenceNumber = sequence
}

// SetRunningBalance records the balance after this entry is applied to its account.
func (e *Entry) SetRunningBalance(balance int64) {
	e.RunningBalance = balance
}

// AddTransactionID associates the entry with its immutable parent transaction.
func (e *Entry) AddTransactionID(transactionID string) {
	e.TransactionID = transactionID
}

func (e *Entry) AddTransnactionID(transactionID string) {
	e.AddTransactionID(transactionID)
}
