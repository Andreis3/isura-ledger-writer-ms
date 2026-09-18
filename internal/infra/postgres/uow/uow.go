package uow

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrBeginTransaction    = errors.New("error opening transaction")
	ErrCommitTransaction   = errors.New("error committing transaction")
	ErrRollbackTransaction = errors.New("error rolling back transaction")
	ErrMaxRetriesExceeded  = errors.New("max retries exceeded due to high concurrency")
)

type UnitOfWork struct {
	begin   func(context.Context) (pgx.Tx, error)
	metrics application.Metrics
}

func NewUnitOfWork(pool *pgxpool.Pool, metrics ...application.Metrics) *UnitOfWork {
	var metric application.Metrics
	if len(metrics) > 0 {
		metric = metrics[0]
	}
	return &UnitOfWork{metrics: metric, begin: func(ctx context.Context) (pgx.Tx, error) {
		return pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	}}
}

func (u *UnitOfWork) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := u.begin(ctx)
	if err != nil {
		return fault.BeginTransactionError(errors.Join(err, ErrBeginTransaction))
	}

	if err := fn(database.WithTx(ctx, tx)); err != nil {
		rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if rbErr := tx.Rollback(rbCtx); rbErr != nil {
			return fault.RollbackTransactionError(errors.Join(err, rbErr, ErrRollbackTransaction))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fault.CommitTransactionError(errors.Join(err, ErrCommitTransaction))
	}

	return nil
}

func (u *UnitOfWork) WithRetryableTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	const maxRetries = 5
	const baseDelay = 10 * time.Millisecond
	const maxDelay = 200 * time.Millisecond

	return retryTransaction(ctx, maxRetries, func(ctx context.Context) error {
		return u.WithTransaction(ctx, fn)
	}, baseDelay, maxDelay, u.metrics)
}

func retryTransaction(
	ctx context.Context,
	maxAttempts int,
	operation func(context.Context) error,
	baseDelay time.Duration,
	maxDelay time.Duration,
	metrics ...application.Metrics,
) error {
	var metric application.Metrics
	if len(metrics) > 0 {
		metric = metrics[0]
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation(ctx)
		if err == nil {
			return nil
		}

		if !isConcurrencyConflict(err) {
			return err
		}

		if attempt == maxAttempts-1 {
			return fault.TransactionConflictError(errors.Join(err, ErrMaxRetriesExceeded))
		}
		if metric != nil {
			metric.RecordConcurrencyRetry()
		}

		backoffLimit := float64(baseDelay) * math.Pow(2, float64(attempt))
		if backoffLimit > float64(maxDelay) {
			backoffLimit = float64(maxDelay)
		}

		sleepDuration := time.Duration(rand.Int64N(int64(backoffLimit)))

		timer := time.NewTimer(sleepDuration)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
	return fault.TransactionConflictError(ErrMaxRetriesExceeded)
}

func isConcurrencyConflict(err error) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "40001" ||
			pgErr.Code == "40P01" ||
			(pgErr.Code == "23505" && pgErr.ConstraintName == "unique_account_sequence")
	}
	return false
}
