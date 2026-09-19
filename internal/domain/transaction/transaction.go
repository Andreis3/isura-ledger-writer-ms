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
	ErrInvalidMaxEntries        = errors.New("maximum entries exceeded")
	ErrDuplicateEntryDirection  = errors.New("duplicate entry direction")
	ErrInvalidTransactionStatus = errors.New("invalid transaction status")
	ErrInvalidDifferentAmount   = errors.New("different amount")
	ErrTransactionNotFound      = errors.New("transaction not found")
	ErrInvalidTransfer          = errors.New("transfer must contain distinct debit and credit entries")
)

type StateMachineStatus map[TransactionStatus][]TransactionStatus

var ValidStateMachine = StateMachineStatus{
	Pending:   []TransactionStatus{Completed, Failed},
	Completed: []TransactionStatus{},
	Failed:    []TransactionStatus{},
}

type TransactionStatus string

const (
	Pending   TransactionStatus = "PENDING"
	Completed TransactionStatus = "COMPLETED"
	Failed    TransactionStatus = "FAILED"
)

func (t TransactionStatus) IsValid() bool {
	switch t {
	case Pending, Completed, Failed:
		return true
	}
	return false
}

type Operation string

const (
	OperationPixIn      Operation = "PIX_IN"
	OperationPixOut     Operation = "PIX_OUT"
	OperationTedIn      Operation = "TED_IN"
	OperationTedOut     Operation = "TED_OUT"
	OperationTransfer   Operation = "TRANSFER"
	OperationDeposit    Operation = "DEPOSIT"
	OperationWithdrawal Operation = "WITHDRAWAL"
	OperationFee        Operation = "FEE"
	OperationRefund     Operation = "REFUND"
)

func (o Operation) IsValid() bool {
	switch o {
	case OperationPixIn, OperationPixOut, OperationTedIn, OperationTedOut,
		OperationTransfer, OperationDeposit, OperationWithdrawal, OperationFee, OperationRefund:
		return true
	}
	return false
}

type TransactionBuilder struct {
	id             entity.ID
	idempotencyKey string
	status         TransactionStatus
	operation      Operation
	entries        []*Entry
	amount         money.Money
	createdAt      time.Time
	updatedAt      time.Time
	fingerprint    string
	metadata       map[string]string
	eval           validator.Evaluator
}

type Transaction struct {
	ID             entity.ID
	IdempotencyKey string
	Status         TransactionStatus
	Operation      Operation
	Amount         money.Money
	Entries        []*Entry
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Fingerprint    string
	Metadata       map[string]string
}

// NewTransactionBuilder initializes a new TransactionBuilder
func NewTransactionBuilder() *TransactionBuilder {
	return &TransactionBuilder{}
}

// WithID sets the transaction ID
func (b *TransactionBuilder) WithID(id ...string) *TransactionBuilder {
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

// WithIdempotencyKey sets the idempotency key
func (b *TransactionBuilder) WithIdempotencyKey(key string) *TransactionBuilder {
	b.eval.CheckField(validator.NotBlank(key), "idempotency_key", "cannot be blank")
	b.eval.CheckField(validator.MaxChars(key, 50), "idempotency_key", "cannot exceed 50 characters")
	b.idempotencyKey = key
	return b
}

// WithStatus sets the transaction status
func (b *TransactionBuilder) WithStatus(status ...TransactionStatus) *TransactionBuilder {
	if len(status) > 0 {
		s := TransactionStatus(status[0])
		b.eval.CheckField(s.IsValid(), "status", "invalid transaction status")
		b.status = s
		return b
	}

	b.status = Pending
	return b
}

// WithAmount sets the transaction amount
func (b *TransactionBuilder) WithAmount(amount money.Money) *TransactionBuilder {
	b.eval.CheckField(!amount.IsZero(), "amount", "cannot be zero")
	b.eval.CheckField(!amount.IsNegative(), "amount", "cannot be negative")
	b.eval.CheckField(amount.Currency().IsValid(), "amount", "invalid currency")
	b.amount = amount
	return b
}

// WithEntries adds entries to the transaction
func (b *TransactionBuilder) WithEntries(entries []*Entry) *TransactionBuilder {
	if len(entries) == 0 {
		return b
	}

	if len(entries) > 2 {
		b.eval.CheckField(false, "entries", "maximum entries exceeded")
		return b
	}

	if len(entries) == 2 {
		if entries[0].Direction == entries[1].Direction {
			b.eval.CheckField(false, "entries", "duplicate entry direction: entries must have opposite directions (debit and credit)")
		}

		if !entries[0].Amount.Equal(entries[1].Amount) {
			b.eval.CheckField(false, "entries", "different amount")
		}
	}

	b.entries = entries
	return b
}

// WithCreatedAt sets the creation time
func (b *TransactionBuilder) WithCreatedAt(createdAt ...time.Time) *TransactionBuilder {
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

// WithUpdatedAt sets the update time
func (b *TransactionBuilder) WithUpdatedAt(updatedAt ...time.Time) *TransactionBuilder {
	if len(updatedAt) > 0 {
		if !updatedAt[0].IsZero() && updatedAt[0].After(time.Now()) {
			b.eval.CheckField(false, "updated_at", "cannot be in the future")
		}
		b.updatedAt = updatedAt[0]
		return b
	}

	b.updatedAt = time.Now()
	return b
}

// WithOperation sets the operation
func (b *TransactionBuilder) WithOperation(operation Operation) *TransactionBuilder {
	b.eval.CheckField(operation.IsValid(), "operation", "invalid transaction operation")
	b.operation = operation
	return b
}

// WithFingerprint associates the canonical request fingerprint with the transaction.
func (b *TransactionBuilder) WithFingerprint(fingerprint string) *TransactionBuilder {
	b.eval.CheckField(len(fingerprint) == 64, "request_fingerprint", "must be a SHA-256 hexadecimal digest")
	b.fingerprint = fingerprint
	return b
}

// WithMetadata stores a copy so callers cannot mutate the aggregate after construction.
func (b *TransactionBuilder) WithMetadata(metadata map[string]string) *TransactionBuilder {
	if metadata == nil {
		return b
	}
	b.metadata = cloneMetadata(metadata)
	return b
}

// Build builds the transaction
func (b *TransactionBuilder) Build() (*Transaction, error) {
	if b.operation == OperationTransfer {
		b.validateTransferEntries()
	}
	if len(b.eval) > 0 {
		return nil, fault.InvalidEntityError(errors.New("invalid transaction entity"), b.eval)
	}

	now := time.Now()

	return &Transaction{
		ID:             b.id,
		IdempotencyKey: b.idempotencyKey,
		Status:         b.status,
		Operation:      b.operation,
		Amount:         b.amount,
		Entries:        b.entries,
		CreatedAt:      shared.CoalesceTime(b.createdAt, now),
		UpdatedAt:      shared.CoalesceTime(b.updatedAt, now),
		Fingerprint:    b.fingerprint,
		Metadata:       cloneMetadata(b.metadata),
	}, nil
}

func (b *TransactionBuilder) validateTransferEntries() {
	if len(b.entries) != 2 {
		b.eval.CheckField(false, "entries", ErrInvalidTransfer.Error())
		return
	}
	first, second := b.entries[0], b.entries[1]
	if first.AccountExternalID == second.AccountExternalID {
		b.eval.CheckField(false, "entries", "debit and credit accounts must be different")
	}
	if first.Direction == second.Direction || !first.Direction.IsValid() || !second.Direction.IsValid() {
		b.eval.CheckField(false, "entries", "entries must contain one debit and one credit")
	}
	if !first.Amount.Equal(second.Amount) {
		b.eval.CheckField(false, "entries", ErrInvalidDifferentAmount.Error())
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

// Complete marks the transaction as completed
func (t *Transaction) Complete() error {
	if !ValidStateMachine.CanTransition(t.Status, Completed) {
		return ErrInvalidTransactionStatus
	}

	t.Status = Completed
	t.UpdatedAt = time.Now()
	return nil
}

// Fail marks the transaction as failed
func (t *Transaction) Fail() error {
	if !ValidStateMachine.CanTransition(t.Status, Failed) {
		return ErrInvalidTransactionStatus
	}

	t.Status = Failed
	t.UpdatedAt = time.Now()
	return nil
}

// CanTransition checks if a transition is allowed
func (s StateMachineStatus) CanTransition(from, to TransactionStatus) bool {
	allowed, ok := s[from]
	if !ok {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}
