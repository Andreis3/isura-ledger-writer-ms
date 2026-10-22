package hold

import (
	"context"
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

var (
	ErrHoldConflict    = errors.New("hold identity conflicts with existing instruction")
	ErrReleaseConflict = errors.New("hold release idempotency key conflicts with existing release")
)

// History records an immutable decision or release applied to an instruction.
type History struct {
	ID, HoldID, EventType                             string
	Requested, AllocatedDelta, Pending, ReleasedDelta money.Money
	PolicyVersion, IdempotencyKey, Reason             string
	ConflictIDs                                       []string
	CreatedAt                                         time.Time
}

// Release records an externally authorized, idempotent release request.
type Release struct {
	ID, HoldID, IdempotencyKey, AuthorizationReference, RequestFingerprint string
	Amount                                                                 money.Money
	CreatedAt                                                              time.Time
}

// Repository persists hold state and its append-only audit records. It never
// owns transaction boundaries; implementations participate in the caller UoW.
type Repository interface {
	Save(ctx context.Context, instruction *Instruction) error
	Update(ctx context.Context, instruction *Instruction) error
	FindByIdentity(ctx context.Context, origin, reference string) (*Instruction, error)
	FindByID(ctx context.Context, id string) (*Instruction, error)
	FindEffectiveByAccount(ctx context.Context, accountID string, currency money.Currency) ([]Instruction, error)
	SaveHistory(ctx context.Context, history History) error
	FindHistory(ctx context.Context, holdID string) ([]History, error)
	FindRelease(ctx context.Context, idempotencyKey string) (*Release, error)
	SaveRelease(ctx context.Context, release Release) error
}
