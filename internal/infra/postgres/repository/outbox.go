package repository

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
)

type OutBoxRepository struct {
	db database.Querier
}

func NewOutBoxRepository(db database.Querier) *OutBoxRepository {
	return &OutBoxRepository{
		db: db,
	}
}

func (r *OutBoxRepository) Save(ctx context.Context, outbox *outbox.Outbox) error {
	db := resolveDB(ctx, r.db)

	query := `INSERT INTO outbox_events (
		id,
		aggregate_id,
		aggregate_type,
		event_type,
        payload,
    	status,
    	attempts,
    	last_attempt_at,
    	created_at,
    	published_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	outboxModel := model.ToOutboxModel(outbox)

	_, err := db.Exec(ctx, query,
		outboxModel.ID,
		outboxModel.AggregateID,
		outboxModel.AggregateType,
		outboxModel.EventType,
		outboxModel.Payload,
		outboxModel.Status,
		outboxModel.Attempts,
		outboxModel.LastAttemptAt,
		outboxModel.CreatedAt,
		outboxModel.PublishedAt,
	)

	return err

}

// ClaimPending atomically reserves a batch before publication. The database
// lock is held only for this UPDATE ... RETURNING statement, never while NATS
// is called.
func (r *OutBoxRepository) ClaimPending(ctx context.Context, limit, maxAttempts int, retryAfter time.Duration) ([]*outbox.Outbox, error) {
	if limit <= 0 || maxAttempts <= 0 {
		return nil, nil
	}

	db := resolveDB(ctx, r.db)
	query := `
	WITH candidates AS (
		SELECT id
		FROM outbox_events
		WHERE attempts < $1
		  AND (
			status = $2
			OR (status = $3 AND last_attempt_at <= now() - $4::interval)
		  )
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $5
	)
	UPDATE outbox_events AS events
	SET status = $3, attempts = events.attempts + 1, last_attempt_at = now()
	FROM candidates
	WHERE events.id = candidates.id
	RETURNING events.id, events.aggregate_id, events.aggregate_type,
		events.event_type, events.payload, events.status, events.attempts,
		events.last_attempt_at, events.created_at, events.published_at`

	rows, err := db.Query(ctx, query, maxAttempts, outbox.Pending, outbox.Failed,
		retryAfter.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	claimed := make([]*outbox.Outbox, 0, limit)
	for rows.Next() {
		var item model.Outbox
		if err := rows.Scan(&item.ID, &item.AggregateID, &item.AggregateType,
			&item.EventType, &item.Payload, &item.Status, &item.Attempts,
			&item.LastAttemptAt, &item.CreatedAt, &item.PublishedAt); err != nil {
			return nil, err
		}
		domainItem, err := model.ToOutboxDomain(item)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, domainItem)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *OutBoxRepository) FindAll(ctx context.Context, status outbox.StatusOutbox, limit int) ([]*outbox.Outbox, error) {
	db := resolveDB(ctx, r.db)

	query := `
	SELECT 
		id,
		aggregate_id,
		aggregate_type,
		event_type,
		payload,
		status,
		attempts,
		last_attempt_at,
		created_at,
		published_at
	FROM outbox_events
	WHERE status = $1
	LIMIT $2
	FOR UPDATE SKIP LOCKED
	`

	rows, err := db.Query(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var outboxes []*outbox.Outbox
	for rows.Next() {
		var outboxModel model.Outbox
		if err := rows.Scan(
			&outboxModel.ID,
			&outboxModel.AggregateID,
			&outboxModel.AggregateType,
			&outboxModel.EventType,
			&outboxModel.Payload,
			&outboxModel.Status,
			&outboxModel.Attempts,
			&outboxModel.LastAttemptAt,
			&outboxModel.CreatedAt,
			&outboxModel.PublishedAt,
		); err != nil {
			return nil, err
		}
		outbox, err := model.ToOutboxDomain(outboxModel)
		if err != nil {
			return nil, err
		}
		outboxes = append(outboxes, outbox)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return outboxes, nil
}
func (r *OutBoxRepository) UpdateOutboxData(ctx context.Context, outboxID entity.ID, data outbox.UpdateOutboxData) error {
	db := resolveDB(ctx, r.db)

	query := `
	UPDATE outbox_events
	SET status = $1, attempts = $2, last_attempt_at = $3, published_at = $4
	WHERE id = $5
	`

	_, err := db.Exec(ctx, query,
		pgtype.Text{String: string(data.Status), Valid: true},
		pgtype.Int2{Int16: int16(data.Attempts), Valid: true},
		database.ToTimestamptz(data.LastAttemptAt),
		database.ToTimestamptz(data.PublishedAt),
		pgtype.Text{String: outboxID.String(), Valid: true},
	)

	return err
}
