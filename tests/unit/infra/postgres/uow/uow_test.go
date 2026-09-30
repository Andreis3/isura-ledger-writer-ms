//go:build unit

package uow_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	uow "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: UOW :: UOW", func() {
	Describe("#RetryTransaction", func() {
		Context("success cases", func() {
			It("should repeat the operation and succeed after a concurrency conflict", func() {
				// Arrange (Given)
				conflict := &pgconn.PgError{Code: "40001", Message: "serialization failure"}
				attempts := 0

				// Act (When)
				err := uow.RetryTransaction(context.Background(), 3, func(context.Context) error {
					attempts++
					if attempts == 1 {
						return fmt.Errorf("transaction attempt: %w", conflict)
					}
					return nil
				}, time.Nanosecond, time.Nanosecond)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(attempts).To(Equal(2))
			})
		})

		Context("error cases", func() {
			It("should return a timeout domain error after exhausting conflict retries", func() {
				// Arrange (Given)
				conflict := &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
				attempts := 0

				// Act (When)
				err := uow.RetryTransaction(context.Background(), 3, func(context.Context) error {
					attempts++
					return conflict
				}, time.Nanosecond, time.Nanosecond)

				// Assert (Then)
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeTimeoutError))
				Expect(attempts).To(Equal(3))
			})

			It("should return a non-conflict error without retrying", func() {
				// Arrange (Given)
				operationErr := errors.New("operation failed")
				attempts := 0

				// Act (When)
				err := uow.RetryTransaction(context.Background(), 3, func(context.Context) error {
					attempts++
					return operationErr
				}, time.Nanosecond, time.Nanosecond)

				// Assert (Then)
				Expect(err).To(MatchError(operationErr))
				Expect(attempts).To(Equal(1))
			})
		})
	})

	Describe("#uow.RetryTransactionWithPolicy", func() {
		Context("success cases", func() {
			It("should apply capped exponential backoff with the configured jitter", func() {
				// Arrange (Given)
				conflict := &pgconn.PgError{Code: "40001"}
				var waits []time.Duration
				policy := uow.RetryPolicy{
					MaxAttempts: 4,
					BaseDelay:   10 * time.Millisecond,
					MaxDelay:    25 * time.Millisecond,
					Jitter:      func(limit time.Duration) time.Duration { return limit / 2 },
					Wait: func(_ context.Context, duration time.Duration) error {
						waits = append(waits, duration)
						return nil
					},
				}

				// Act (When)
				err := uow.RetryTransactionWithPolicy(context.Background(), policy, func(context.Context) error {
					return conflict
				})

				// Assert (Then)
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeTimeoutError))
				Expect(waits).To(Equal([]time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 12500 * time.Microsecond}))
			})
		})

		Context("error cases", func() {
			It("should stop retrying when the context is canceled", func() {
				// Arrange (Given)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				attempts := 0
				policy := uow.RetryPolicy{
					MaxAttempts: 3,
					BaseDelay:   time.Second,
					MaxDelay:    time.Second,
					Jitter:      func(time.Duration) time.Duration { return time.Second },
					Wait: func(ctx context.Context, _ time.Duration) error {
						cancel()
						return ctx.Err()
					},
				}

				// Act (When)
				err := uow.RetryTransactionWithPolicy(ctx, policy, func(context.Context) error {
					attempts++
					return &pgconn.PgError{Code: "40001"}
				})

				// Assert (Then)
				Expect(err).To(MatchError(context.Canceled))
				Expect(attempts).To(Equal(1))
			})
		})
	})

	Describe("#RetryBackoff", func() {
		Context("success cases", func() {
			It("should double delays up to the configured maximum", func() {
				// Arrange (Given)

				// Act (When)
				initial := uow.RetryBackoff(10*time.Millisecond, 200*time.Millisecond, 0)
				doubled := uow.RetryBackoff(10*time.Millisecond, 200*time.Millisecond, 2)
				capped := uow.RetryBackoff(10*time.Millisecond, 200*time.Millisecond, 8)

				// Assert (Then)
				Expect(initial).To(Equal(10 * time.Millisecond))
				Expect(doubled).To(Equal(40 * time.Millisecond))
				Expect(capped).To(Equal(200 * time.Millisecond))
			})
		})

		Context("error cases", func() {
			It("should return zero when either duration is invalid", func() {
				// Arrange (Given)

				// Act (When)
				zeroBase := uow.RetryBackoff(0, time.Second, 1)
				zeroMaximum := uow.RetryBackoff(time.Second, 0, 1)

				// Assert (Then)
				Expect(zeroBase).To(BeZero())
				Expect(zeroMaximum).To(BeZero())
			})
		})
	})

	Describe("#WithTransaction", func() {
		Context("error cases", func() {
			It("should report operation and rollback errors", func() {
				// Arrange (Given)
				operationErr := errors.New("operation failed")
				rollbackErr := errors.New("rollback failed")
				tx := &transactionStub{rollbackErr: rollbackErr}
				sut := uow.NewUnitOfWorkWithBegin(func(context.Context) (pgx.Tx, error) { return tx, nil })

				// Act (When)
				err := sut.WithTransaction(context.Background(), func(context.Context) error { return operationErr })

				// Assert (Then)
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeDatabaseError))
				Expect(errors.Is(err, operationErr)).To(BeTrue())
				Expect(errors.Is(err, rollbackErr)).To(BeTrue())
				Expect(tx.rollbackCalls).To(Equal(1))
			})
		})
	})

	Describe("#IsConcurrencyConflict", func() {
		Context("success cases", func() {
			It("should classify supported PostgreSQL concurrency errors", func() {
				// Arrange (Given)
				cases := []struct {
					name      string
					err       error
					retryable bool
				}{
					{name: "serialization", err: &pgconn.PgError{Code: "40001"}, retryable: true},
					{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, retryable: true},
					{name: "entry sequence number", err: &pgconn.PgError{Code: "23505", ConstraintName: "unique_entry_sequence_number"}, retryable: true},
					{name: "idempotency collision", err: &pgconn.PgError{Code: "23505", ConstraintName: "transactions_idempotency_key_key"}},
					{name: "other unique constraint", err: &pgconn.PgError{Code: "23505", ConstraintName: "accounts_account_number_key"}},
				}

				// Act (When) and Assert (Then)
				for _, testCase := range cases {
					Expect(uow.IsConcurrencyConflict(testCase.err)).To(Equal(testCase.retryable), testCase.name)
				}
			})
		})
	})
})

type transactionStub struct {
	pgx.Tx
	rollbackErr   error
	rollbackCalls int
}

func (t *transactionStub) Rollback(context.Context) error {
	t.rollbackCalls++
	return t.rollbackErr
}
