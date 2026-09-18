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

	err := retryTransaction(context.Background(), 2, func(context.Context) error {
		attempts++
		return conflict
	}, time.Nanosecond, time.Nanosecond)

	var domainErr *fault.DomainError
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, fault.CodeTimeoutError, domainErr.Code)
	require.Equal(t, 2, attempts)
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
		{name: "account sequence", err: &pgconn.PgError{Code: "23505", ConstraintName: "unique_account_sequence"}, retryable: true},
		{name: "idempotency collision", err: &pgconn.PgError{Code: "23505", ConstraintName: "transactions_idempotency_key_key"}, retryable: false},
		{name: "other unique constraint", err: &pgconn.PgError{Code: "23505", ConstraintName: "accounts_account_number_key"}, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.retryable, isConcurrencyConflict(tt.err))
		})
	}
}
