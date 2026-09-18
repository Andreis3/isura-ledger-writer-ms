package outbox

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
)

type UpdateOutboxData struct {
	Status        StatusOutbox
	Attempts      int
	LastAttemptAt *time.Time
	PublishedAt   *time.Time
}
type Repository interface {
	Save(ctx context.Context, outbox *Outbox) error
	ClaimPending(ctx context.Context, limit, maxAttempts int, retryAfter time.Duration) ([]*Outbox, error)
	FindAll(ctx context.Context, status StatusOutbox, limit int) ([]*Outbox, error)
	UpdateOutboxData(ctx context.Context, outboxID entity.ID, data UpdateOutboxData) error
}
