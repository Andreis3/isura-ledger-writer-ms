package outbox

import (
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

const MaxAttempts = 3

var (
	ErrMaxAttemptsExceeded = errors.New("max attempts exceeded")
	ErrTransitionStatus    = errors.New("transition status")
)

type AggregateType string

type EventType string

type StatusOutbox string

const (
	Transaction AggregateType = "TRANSACTION"
)

const (
	TransactionCreated EventType = "ledger.transaction.created"
	BalanceUpdated     EventType = "ledger.balance.updated"
	EntryCreated       EventType = "ledger.entry.created"
)

const (
	Pending StatusOutbox = "PENDING"
	Failed  StatusOutbox = "FAILED"
	Success StatusOutbox = "SUCCESS"
)

type StateMachineStatus map[StatusOutbox][]StatusOutbox

var validStateMachine = StateMachineStatus{
	Pending: []StatusOutbox{Failed, Success},
	Failed:  []StatusOutbox{Pending},
	Success: []StatusOutbox{},
}

type Outbox struct {
	ID            entity.ID
	AggregateID   string
	AggregateType AggregateType
	EventType     EventType
	Payload       []byte
	Status        StatusOutbox
	Attempts      int
	LastAttemptAt *time.Time
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

type OutboxBuilder struct {
	id            entity.ID
	aggregateID   string
	aggregateType AggregateType
	eventType     EventType
	payload       []byte
	status        StatusOutbox
	attempts      int
	lastAttemptAt *time.Time
	createdAt     time.Time
	publishedAt   *time.Time
	eval          validator.Evaluator
}

func NewOutboxBuilder() *OutboxBuilder {
	return &OutboxBuilder{}
}

// WithID sets the outbox ID
func (b *OutboxBuilder) WithID(id ...string) *OutboxBuilder {
	if len(id) > 0 {
		b.eval.CheckField(validator.NotBlank(id[0]), "id", "cannot be blank")
		b.eval.CheckField(validator.MatchesUUID(id[0]), "id", "is not uuid")
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

// WithAggregateID sets the aggregate
func (b *OutboxBuilder) WithAggregateID(aggregateID string) *OutboxBuilder {
	b.eval.CheckField(validator.NotBlank(aggregateID), "aggregate_id", "cannot be blank")
	b.aggregateID = aggregateID
	return b
}

// WithAggregateType sets the aggregate type
func (b *OutboxBuilder) WithAggregateType(aggregateType ...AggregateType) *OutboxBuilder {
	if len(aggregateType) > 0 {
		b.eval.CheckField(validator.NotBlank(string(aggregateType[0])), "aggregate_type", "cannot be blank")
		b.eval.CheckField(aggregateType[0].IsValid(), "aggregate_type", "invalid aggregate type")
		b.aggregateType = AggregateType(aggregateType[0])
		return b
	}

	b.aggregateType = Transaction
	return b
}

// WithEventType sets the event type
func (b *OutboxBuilder) WithEventType(eventType ...EventType) *OutboxBuilder {
	if len(eventType) > 0 {
		b.eval.CheckField(validator.NotBlank(string(eventType[0])), "event_type", "cannot be blank")
		b.eval.CheckField(eventType[0].IsValid(), "event_type", "event type is invalid")
		b.eventType = EventType(eventType[0])
		return b
	}

	b.eventType = TransactionCreated
	return b
}

// WithStatus sets the status
func (b *OutboxBuilder) WithStatus(status ...StatusOutbox) *OutboxBuilder {
	if len(status) > 0 {
		b.eval.CheckField(validator.NotBlank(string(status[0])), "status", "cannot be blank")
		b.eval.CheckField(status[0].IsValid(), "status", "invalid status")
		b.status = StatusOutbox(status[0])
		return b
	}

	b.status = Pending
	return b
}

// WithAttempts sets the number of attempts
func (b *OutboxBuilder) WithAttempts(attemps ...int) *OutboxBuilder {
	if len(attemps) > 0 {
		b.attempts = attemps[0]
		return b
	}

	b.attempts = 0
	return b
}

// WithLastAttemptAt sets the last attempt time
func (b *OutboxBuilder) WithLastAttemptAt(lastAttemptAt ...time.Time) *OutboxBuilder {
	if len(lastAttemptAt) > 0 {
		if !lastAttemptAt[0].IsZero() && lastAttemptAt[0].After(time.Now()) {
			b.eval.CheckField(false, "last_attempt_at", "cannot be in the future")
		}
		b.lastAttemptAt = &lastAttemptAt[0]
		return b
	}

	b.lastAttemptAt = nil
	return b
}

// WithCreatedAt sets the creation time
func (b *OutboxBuilder) WithCreatedAt(createdAt ...time.Time) *OutboxBuilder {
	if len(createdAt) > 0 {
		if !createdAt[0].IsZero() && createdAt[0].After(time.Now()) {
			b.eval.CheckField(false, "created_at", "cannot be in the future")
		}
		b.createdAt = createdAt[0]
		return b
	}

	b.createdAt = time.Now()
	return b
}

// WithPublishedAt sets the published time
func (b *OutboxBuilder) WithPublishedAt(publishedAt ...time.Time) *OutboxBuilder {
	if len(publishedAt) > 0 {
		if !publishedAt[0].IsZero() && publishedAt[0].After(time.Now()) {
			b.eval.CheckField(false, "published_at", "cannot be in the future")
		}
		b.publishedAt = &publishedAt[0]
		return b
	}

	b.publishedAt = nil
	return b
}

// WithPayload sets the payload
func (b *OutboxBuilder) WithPayload(payload []byte) *OutboxBuilder {
	b.payload = payload
	return b
}

// Build builds the outbox
func (b *OutboxBuilder) Build() (*Outbox, error) {
	if len(b.eval) > 0 {
		return nil, fault.InvalidEntityError(errors.New("invalid outbox entity"), b.eval)
	}

	return &Outbox{
		ID:            b.id,
		AggregateID:   b.aggregateID,
		AggregateType: b.aggregateType,
		EventType:     b.eventType,
		Payload:       b.payload,
		Status:        b.status,
		Attempts:      b.attempts,
		LastAttemptAt: b.lastAttemptAt,
		CreatedAt:     b.createdAt,
		PublishedAt:   b.publishedAt,
	}, nil
}

func NewOutbox(transctionID string, payload []byte) (*Outbox, error) {
	return NewOutboxBuilder().
		WithID().
		WithAggregateID(transctionID).
		WithAggregateType().
		WithEventType().
		WithStatus().
		WithAttempts().
		WithLastAttemptAt().
		WithCreatedAt().
		WithPayload(payload).
		Build()
}

func (a AggregateType) IsValid() bool {
	switch a {
	case Transaction:
		return true
	}
	return false
}

func (o *Outbox) Publish() error {
	if !validStateMachine.CanTransition(o.Status, Success) {
		return ErrTransitionStatus
	}
	o.Status = Success
	o.PublishedAt = new(time.Now())
	return nil
}

func (o *Outbox) MarkFailed() error {
	if !validStateMachine.CanTransition(o.Status, Failed) {
		return ErrTransitionStatus
	}

	o.Status = Failed
	o.Attempts++
	o.LastAttemptAt = new(time.Now())

	return nil
}

func (o *Outbox) Retry() error {
	if o.Attempts >= MaxAttempts {
		return ErrMaxAttemptsExceeded
	}

	if !validStateMachine.CanTransition(o.Status, Pending) {
		return ErrTransitionStatus
	}

	o.Status = Pending

	return nil
}

func (s StatusOutbox) IsValid() bool {
	switch s {
	case Pending, Failed, Success:
		return true
	}
	return false
}

func (e EventType) IsValid() bool {
	switch e {
	case TransactionCreated, BalanceUpdated, EntryCreated:
		return true
	}
	return false
}

func (s StateMachineStatus) CanTransition(from, to StatusOutbox) bool {
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
