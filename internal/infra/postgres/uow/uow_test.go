//go:build unit
// +build unit

package uow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestRetryTransactionRepeatsTheWholeOperationAfterAConflict(t *testing.T) {
	t.Parallel()

	conflict := &pgconn.PgError{Code: "40001", Message: "serialization failure"}
	attempts := 0

	err := retryTransaction(context.Background(), 3, func(context.Context) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("transaction attempt: %w", conflict)
		}
		return nil
	}, time.Nanosecond, time.Nanosecond)

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestRetryTransactionRetriesDeadlockAndCountsAllAttempts(t *testing.T) {
	t.Parallel()

	conflict := &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
	attempts := 0

	err := retryTransaction(context.Background(), 3, func(context.Context) error {
		attempts++
		return conflict
	}, time.Nanosecond, time.Nanosecond)

	var domainErr *fault.DomainError
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, fault.CodeTimeoutError, domainErr.Code)
	require.Equal(t, 3, attempts)
}

func TestRetryTransactionDoesNotTurnRollbackFailureIntoSuccess(t *testing.T) {
	t.Parallel()

	operationError := errors.New("rollback failed")
	attempts := 0

	err := retryTransaction(context.Background(), 3, func(context.Context) error {
		attempts++
		return operationError
	}, time.Nanosecond, time.Nanosecond)

	require.ErrorIs(t, err, operationError)
	require.Equal(t, 1, attempts)
}

func TestRetryTransactionReturnsTimeoutCodeAfterConflictLimit(t *testing.T) {
	t.Parallel()

	conflict := &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
	attempts := 0

	err := retryTransaction(context.Background(), 5, func(context.Context) error {
		attempts++
		return conflict
	}, time.Nanosecond, time.Nanosecond)

	var domainErr *fault.DomainError
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, fault.CodeTimeoutError, domainErr.Code)
	require.Equal(t, 5, attempts)
}

func TestRetryTransactionUsesExponentialCappedJitter(t *testing.T) {
	t.Parallel()

	conflict := &pgconn.PgError{Code: "40001"}
	var waits []time.Duration
	policy := retryPolicy{
		maxAttempts: 4,
		baseDelay:   10 * time.Millisecond,
		maxDelay:    25 * time.Millisecond,
		jitter: func(limit time.Duration) time.Duration {
			return limit / 2
		},
		wait: func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			return nil
		},
	}

	err := retryTransactionWithPolicy(context.Background(), policy, func(context.Context) error {
		return conflict
	})

	var domainErr *fault.DomainError
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		12500 * time.Microsecond,
	}, waits)
}

func TestRetryTransactionStopsBeforeNextAttemptWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	conflict := &pgconn.PgError{Code: "40001"}
	attempts := 0

	err := retryTransactionWithPolicy(ctx, retryPolicy{
		maxAttempts: 3,
		baseDelay:   time.Second,
		maxDelay:    time.Second,
		jitter:      func(time.Duration) time.Duration { return time.Second },
		wait: func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	}, func(context.Context) error {
		attempts++
		return conflict
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestRetryBackoffIsCappedAndHandlesInvalidDurations(t *testing.T) {
	t.Parallel()

	require.Equal(t, 10*time.Millisecond, retryBackoff(10*time.Millisecond, 200*time.Millisecond, 0))
	require.Equal(t, 40*time.Millisecond, retryBackoff(10*time.Millisecond, 200*time.Millisecond, 2))
	require.Equal(t, 200*time.Millisecond, retryBackoff(10*time.Millisecond, 200*time.Millisecond, 8))
	require.Zero(t, retryBackoff(0, time.Second, 1))
	require.Zero(t, retryBackoff(time.Second, 0, 1))
}

func TestWithTransactionRollbackFailureTakesPrecedence(t *testing.T) {
	t.Parallel()

	operationErr := errors.New("operation failed")
	rollbackErr := errors.New("rollback failed")
	tx := &transactionStub{rollbackErr: rollbackErr}
	sut := &UnitOfWork{begin: func(context.Context) (pgx.Tx, error) {
		return tx, nil
	}}

	err := sut.WithTransaction(context.Background(), func(context.Context) error {
		return operationErr
	})

	var domainErr *fault.DomainError
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, fault.CodeDatabaseError, domainErr.Code)
	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, rollbackErr)
	require.Equal(t, 1, tx.rollbackCalls)
}

func TestIsConcurrencyConflictOnlyClassifiesSupportedPostgresErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, retryable: true},
		{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, retryable: true},
		{name: "entry sequence number", err: &pgconn.PgError{Code: "23505", ConstraintName: "unique_entry_sequence_number"}, retryable: true},
		{name: "idempotency collision", err: &pgconn.PgError{Code: "23505", ConstraintName: "transactions_idempotency_key_key"}, retryable: false},
		{name: "other unique constraint", err: &pgconn.PgError{Code: "23505", ConstraintName: "accounts_account_number_key"}, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.retryable, isConcurrencyConflict(tt.err))
		})
	}
}

type transactionStub struct {
	pgx.Tx
	rollbackErr   error
	rollbackCalls int
}

func (t *transactionStub) Rollback(context.Context) error {
	t.rollbackCalls++
	return t.rollbackErr
}
