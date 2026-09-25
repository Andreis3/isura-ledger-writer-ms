package uow

import (
	"context"
	"errors"
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

type retryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	jitter      func(time.Duration) time.Duration
	wait        func(context.Context, time.Duration) error
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
	policy := retryPolicy{
		maxAttempts: 5,
		baseDelay:   10 * time.Millisecond,
		maxDelay:    200 * time.Millisecond,
		jitter: func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit)))
		},
		wait: waitForRetry,
	}

	return retryTransactionWithPolicy(ctx, policy, func(ctx context.Context) error {
		return u.WithTransaction(ctx, fn)
	}, u.metrics)
}

func retryTransaction(
	ctx context.Context,
	maxAttempts int,
	operation func(context.Context) error,
	baseDelay time.Duration,
	maxDelay time.Duration,
	metrics ...application.Metrics,
) error {
	return retryTransactionWithPolicy(ctx, retryPolicy{
		maxAttempts: maxAttempts,
		baseDelay:   baseDelay,
		maxDelay:    maxDelay,
		jitter: func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit)))
		},
		wait: waitForRetry,
	}, operation, metrics...)
}

func retryTransactionWithPolicy(
	ctx context.Context,
	policy retryPolicy,
	operation func(context.Context) error,
	metrics ...application.Metrics,
) error {
	var metric application.Metrics
	if len(metrics) > 0 {
		metric = metrics[0]
	}
	for attempt := 0; attempt < policy.maxAttempts; attempt++ {
		err := operation(ctx)
		if err == nil {
			return nil
		}

		if !isConcurrencyConflict(err) {
			return err
		}

		if attempt == policy.maxAttempts-1 {
			return fault.TransactionConflictError(errors.Join(err, ErrMaxRetriesExceeded))
		}
		if metric != nil {
			metric.RecordConcurrencyRetry()
		}

		backoffLimit := retryBackoff(policy.baseDelay, policy.maxDelay, attempt)
		if err := policy.wait(ctx, policy.jitter(backoffLimit)); err != nil {
			return err
		}
	}
	return fault.TransactionConflictError(ErrMaxRetriesExceeded)
}

func retryBackoff(baseDelay, maxDelay time.Duration, attempt int) time.Duration {
	if baseDelay <= 0 || maxDelay <= 0 {
		return 0
	}
	backoff := baseDelay
	for index := 0; index < attempt && backoff < maxDelay; index++ {
		if backoff > maxDelay/2 {
			return maxDelay
		}
		backoff *= 2
	}
	if backoff > maxDelay {
		return maxDelay
	}
	return backoff
}

func waitForRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isConcurrencyConflict(err error) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "40001" ||
			pgErr.Code == "40P01" ||
			(pgErr.Code == "23505" && pgErr.ConstraintName == "unique_entry_sequence_number")
	}
	return false
}
