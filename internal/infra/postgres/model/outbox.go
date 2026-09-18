package model

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
)

type Outbox struct {
	ID            pgtype.Text
	AggregateID   pgtype.Text
	AggregateType pgtype.Text
	EventType     pgtype.Text
	Payload       []byte
	Status        pgtype.Text
	Attempts      pgtype.Int2
	LastAttemptAt pgtype.Timestamptz
	PublishedAt   pgtype.Timestamptz
	CreatedAt     pgtype.Timestamptz
}

func ToOutboxModel(domain *outbox.Outbox) Outbox {
	return Outbox{
		ID: pgtype.Text{
			String: domain.ID.String(),
			Valid:  true,
		},
		Status: pgtype.Text{
			String: string(domain.Status),
			Valid:  true,
		},
		AggregateID: pgtype.Text{
			String: string(domain.AggregateID),
			Valid:  true,
		},
		AggregateType: pgtype.Text{
			String: string(domain.AggregateType),
			Valid:  true,
		},
		Attempts: pgtype.Int2{
			Int16: int16(domain.Attempts),
			Valid: true,
		},
		EventType: pgtype.Text{
			String: string(domain.EventType),
			Valid:  true,
		},
		Payload:       domain.Payload,
		LastAttemptAt: database.ToTimestamptz(domain.LastAttemptAt),
		CreatedAt: pgtype.Timestamptz{
			Time:  domain.CreatedAt,
			Valid: true,
		},
		PublishedAt: database.ToTimestamptz(domain.PublishedAt),
	}
}

func ToOutboxDomain(model Outbox) (*outbox.Outbox, error) {
	id, err := entity.NewID(model.ID.String)
	if err != nil {
		return nil, err
	}

	builder := outbox.NewOutboxBuilder().
		WithID(id.String()).
		WithAggregateID(model.AggregateID.String).
		WithAggregateType(outbox.AggregateType(model.AggregateType.String)).
		WithEventType(outbox.EventType(model.EventType.String)).
		WithPayload(model.Payload).
		WithStatus(outbox.StatusOutbox(model.Status.String)).
		WithAttempts(int(model.Attempts.Int16)).
		WithCreatedAt(model.CreatedAt.Time)
	if lastAttemptAt := database.ToTimePtr(model.LastAttemptAt); lastAttemptAt != nil {
		builder.WithLastAttemptAt(*lastAttemptAt)
	}
	if publishedAt := database.ToTimePtr(model.PublishedAt); publishedAt != nil {
		builder.WithPublishedAt(*publishedAt)
	}
	return builder.Build()

}
