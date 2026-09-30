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

// RetryPolicy configures attempts, backoff, jitter, and waiting for retryable transactions.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      func(time.Duration) time.Duration
	Wait        func(context.Context, time.Duration) error
}

func NewUnitOfWork(pool *pgxpool.Pool, metrics ...application.Metrics) *UnitOfWork {
	return NewUnitOfWorkWithBegin(func(ctx context.Context) (pgx.Tx, error) {
		return pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	}, metrics...)
}

// NewUnitOfWorkWithBegin creates a unit of work using the supplied transaction starter.
func NewUnitOfWorkWithBegin(begin func(context.Context) (pgx.Tx, error), metrics ...application.Metrics) *UnitOfWork {
	var metric application.Metrics
	if len(metrics) > 0 {
		metric = metrics[0]
	}
	return &UnitOfWork{metrics: metric, begin: begin}
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
	policy := RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		Jitter: func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit)))
		},
		Wait: waitForRetry,
	}

	return RetryTransactionWithPolicy(ctx, policy, func(ctx context.Context) error {
		return u.WithTransaction(ctx, fn)
	}, u.metrics)
}

// RetryTransaction retries an operation when it fails with a supported concurrency conflict.
func RetryTransaction(
	ctx context.Context,
	maxAttempts int,
	operation func(context.Context) error,
	baseDelay time.Duration,
	maxDelay time.Duration,
	metrics ...application.Metrics,
) error {
	return RetryTransactionWithPolicy(ctx, RetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   baseDelay,
		MaxDelay:    maxDelay,
		Jitter: func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit)))
		},
		Wait: waitForRetry,
	}, operation, metrics...)
}

// RetryTransactionWithPolicy retries an operation according to policy.
func RetryTransactionWithPolicy(
	ctx context.Context,
	policy RetryPolicy,
	operation func(context.Context) error,
	metrics ...application.Metrics,
) error {
	var metric application.Metrics
	if len(metrics) > 0 {
		metric = metrics[0]
	}
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		err := operation(ctx)
		if err == nil {
			return nil
		}

		if !IsConcurrencyConflict(err) {
			return err
		}

		if attempt == policy.MaxAttempts-1 {
			return fault.TransactionConflictError(errors.Join(err, ErrMaxRetriesExceeded))
		}
		if metric != nil {
			metric.RecordConcurrencyRetry()
		}

		backoffLimit := RetryBackoff(policy.BaseDelay, policy.MaxDelay, attempt)
		if err := policy.Wait(ctx, policy.Jitter(backoffLimit)); err != nil {
			return err
		}
	}
	return fault.TransactionConflictError(ErrMaxRetriesExceeded)
}

// RetryBackoff returns the exponential backoff delay for an attempt, capped at maxDelay.
func RetryBackoff(baseDelay, maxDelay time.Duration, attempt int) time.Duration {
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

// IsConcurrencyConflict reports whether err is a supported PostgreSQL concurrency conflict.
func IsConcurrencyConflict(err error) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "40001" ||
			pgErr.Code == "40P01" ||
			(pgErr.Code == "23505" && pgErr.ConstraintName == "unique_entry_sequence_number")
	}
	return false
}
