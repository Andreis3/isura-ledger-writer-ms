package transaction

import (
	"time"
)

type TransactionCreated struct {
	EventVersion    int                          `json:"event_version,omitempty"`
	EventID         string                       `json:"event_id"`
	TransactionID   string                       `json:"transaction_id"`
	IdempotencyKey  string                       `json:"idempotency_key"`
	DebitAccountID  string                       `json:"debit_account_id,omitempty"`
	CreditAccountID string                       `json:"credit_account_id,omitempty"`
	Amount          int64                        `json:"amount"`
	Currency        string                       `json:"currency"`
	Status          string                       `json:"status"`
	OccurredAt      time.Time                    `json:"occurred_at"`
	Metadata        map[string]string            `json:"metadata,omitempty"`
	Entries         []TransactionEntryCreated    `json:"entries"`
	Movements       []TransactionMovementCreated `json:"movements"`
}

// TransactionEntryCreated is one ordered posting included in a created transaction event.
type TransactionEntryCreated struct {
	EntryID        string    `json:"entry_id"`
	Position       int64     `json:"position"`
	AccountID      string    `json:"account_id"`
	SequenceNumber int64     `json:"sequence_number"`
	Direction      Direction `json:"direction"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	RunningBalance int64     `json:"running_balance"`
	MovementID     string    `json:"movement_id"`
}

// TransactionMovementCreated describes the entries grouped by one movement.
type TransactionMovementCreated struct {
	MovementID  string   `json:"movement_id"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	EntryIDs    []string `json:"entry_ids"`
}

func NewTransactionCreated() *TransactionCreated {
	return &TransactionCreated{}
}

// WithEventID sets the outbox event identifier used for publication deduplication.
func (t *TransactionCreated) WithEventID(eventID string) *TransactionCreated {
	t.EventID = eventID
	return t
}

func (t *TransactionCreated) WithTransactionID(transactionID string) *TransactionCreated {
	t.TransactionID = transactionID
	return t
}

func (t *TransactionCreated) WithIdempotencyKey(idempotencyKey string) *TransactionCreated {
	t.IdempotencyKey = idempotencyKey
	return t
}

func (t *TransactionCreated) WithDebitAccountID(debitAccountID string) *TransactionCreated {
	t.DebitAccountID = debitAccountID
	return t
}

func (t *TransactionCreated) WithCreditAccountID(creditAccountID string) *TransactionCreated {
	t.CreditAccountID = creditAccountID
	return t
}

func (t *TransactionCreated) WithAmount(amount int64) *TransactionCreated {
	t.Amount = amount
	return t
}

func (t *TransactionCreated) WithCurrency(currency string) *TransactionCreated {
	t.Currency = currency
	return t
}

func (t *TransactionCreated) WithStatus(status string) *TransactionCreated {
	t.Status = status
	return t
}

func (t *TransactionCreated) WithOccurredAt(occurredAt time.Time) *TransactionCreated {
	t.OccurredAt = occurredAt
	return t
}

// WithMetadata stores a copy of the event metadata.
func (t *TransactionCreated) WithMetadata(metadata map[string]string) *TransactionCreated {
	t.Metadata = cloneMetadata(metadata)
	return t
}

// WithEntries stores a copy of the ordered transaction composition.
func (t *TransactionCreated) WithEntries(entries []TransactionEntryCreated) *TransactionCreated {
	t.Entries = append([]TransactionEntryCreated(nil), entries...)
	return t
}

func (t *TransactionCreated) Build() *TransactionCreated {
	return &TransactionCreated{
		EventVersion:    t.EventVersion,
		EventID:         t.EventID,
		TransactionID:   t.TransactionID,
		IdempotencyKey:  t.IdempotencyKey,
		DebitAccountID:  t.DebitAccountID,
		CreditAccountID: t.CreditAccountID,
		Amount:          t.Amount,
		Currency:        t.Currency,
		Status:          t.Status,
		OccurredAt:      t.OccurredAt,
		Metadata:        cloneMetadata(t.Metadata),
		Entries:         append([]TransactionEntryCreated(nil), t.Entries...),
		Movements:       cloneCreatedMovements(t.Movements),
	}
}

func TransactionCreatedFacade(entityTransaction Transaction, idempotencyKey string, debitAccountID string, creditAccountID string) *TransactionCreated {
	entries := make([]TransactionEntryCreated, 0, len(entityTransaction.Entries))
	entryMovementIDs := make(map[string]string, len(entityTransaction.Entries))
	movements := make([]TransactionMovementCreated, 0, len(entityTransaction.Movements))
	for _, movement := range entityTransaction.Movements {
		entryIDs := make([]string, 0, len(movement.Entries))
		for _, entry := range movement.Entries {
			entryID := entry.ID.String()
			entryIDs = append(entryIDs, entryID)
			entryMovementIDs[entryID] = movement.ID.String()
		}
		movements = append(movements, TransactionMovementCreated{
			MovementID: movement.ID.String(), Type: movement.Type,
			Description: movement.Description, EntryIDs: entryIDs,
		})
	}
	for _, entry := range entityTransaction.Entries {
		entries = append(entries, TransactionEntryCreated{
			EntryID: entry.ID.String(), Position: entry.TransactionPosition,
			AccountID: entry.AccountID, SequenceNumber: entry.SequenceNumber,
			Direction: entry.Direction, Amount: entry.Amount.Amount(),
			Currency: string(entry.Amount.Currency()), RunningBalance: entry.RunningBalance,
			MovementID: entryMovementIDs[entry.ID.String()],
		})
	}

	return NewTransactionCreated().
		WithEventVersion(1).
		WithTransactionID(entityTransaction.ID.String()).
		WithIdempotencyKey(idempotencyKey).
		WithDebitAccountID(debitAccountID).
		WithCreditAccountID(creditAccountID).
		WithAmount(entityTransaction.Amount.Amount()).
		WithCurrency(string(entityTransaction.Amount.Currency())).
		WithStatus(string(entityTransaction.Status)).
		WithOccurredAt(time.Now()).
		WithMetadata(entityTransaction.Metadata).
		WithEntries(entries).
		WithMovements(movements).
		Build()
}

// WithEventVersion sets the payload schema version.
func (t *TransactionCreated) WithEventVersion(version int) *TransactionCreated {
	t.EventVersion = version
	return t
}

// WithMovements stores a copy of the movement snapshot.
func (t *TransactionCreated) WithMovements(movements []TransactionMovementCreated) *TransactionCreated {
	t.Movements = cloneCreatedMovements(movements)
	return t
}

func cloneCreatedMovements(movements []TransactionMovementCreated) []TransactionMovementCreated {
	cloned := make([]TransactionMovementCreated, len(movements))
	for index, movement := range movements {
		cloned[index] = movement
		cloned[index].EntryIDs = append([]string(nil), movement.EntryIDs...)
	}
	return cloned
}
