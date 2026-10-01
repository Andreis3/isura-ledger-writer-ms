package repository

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/stretchr/testify/mock"
)

type OutboxRepositoryMock struct{ mock.Mock }

func (m *OutboxRepositoryMock) Save(ctx context.Context, item *outbox.Outbox) error {
	return m.Called(ctx, item).Error(0)
}

func (m *OutboxRepositoryMock) ClaimPending(ctx context.Context, limit, maxAttempts int, retryAfter time.Duration) ([]*outbox.Outbox, error) {
	args := m.Called(ctx, limit, maxAttempts, retryAfter)
	var items []*outbox.Outbox
	if value := args.Get(0); value != nil {
		items = value.([]*outbox.Outbox)
	}
	return items, args.Error(1)
}

func (m *OutboxRepositoryMock) FindAll(ctx context.Context, status outbox.StatusOutbox, limit int) ([]*outbox.Outbox, error) {
	args := m.Called(ctx, status, limit)
	var items []*outbox.Outbox
	if value := args.Get(0); value != nil {
		items = value.([]*outbox.Outbox)
	}
	return items, args.Error(1)
}

func (m *OutboxRepositoryMock) UpdateOutboxData(ctx context.Context, id entity.ID, data outbox.UpdateOutboxData) error {
	return m.Called(ctx, id, data).Error(0)
}

var _ outbox.Repository = (*OutboxRepositoryMock)(nil)
