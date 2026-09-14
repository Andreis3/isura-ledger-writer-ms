package uow

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
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
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

func (u *UnitOfWork) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fault.BeginTransactionError(errors.Join(err, ErrBeginTransaction))
	}

	if err := fn(database.WithTx(ctx, tx)); err != nil {
		rbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := u.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}

		if !isConcurrencyConflict(err) {
			return err
		}

		if attempt == maxRetries-1 {
			return fault.ConflictError(errors.Join(err, ErrMaxRetriesExceeded))
		}

		backoffLimit := float64(baseDelay) * math.Pow(2, float64(attempt))
		if backoffLimit > float64(maxDelay) {
			backoffLimit = float64(maxDelay)
		}

		sleepDuration := time.Duration(rand.Int64N(int64(backoffLimit)))

		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func isConcurrencyConflict(err error) bool {
	// Correção: Extrai o ponteiro do erro tipado tratando unwrapping de forma segura via Go nativo
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		// Altere para a sua constraint de concorrência do Ledger (ex: unique_account_sequence)
		if pgErr.Code == "23505" && (pgErr.ConstraintName == "unique_account_sequence" ||
			strings.Contains(pgErr.ConstraintName, "idempotency_key")) {
			return true
		}
	}
	return false
}
