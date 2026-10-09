//go:build unit

package uow_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func noDelayRetryPolicy(maxAttempts int) uow.RetryPolicy {
	return uow.RetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   time.Millisecond,
		MaxDelay:    time.Millisecond,
		Jitter: func(time.Duration) time.Duration {
			return 0
		},
		Wait: func(context.Context, time.Duration) error {
			return nil
		},
	}
}

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: UOW :: UOW", func() {
	Describe("#RetryTransactionWithPolicy", func() {
		Context("success cases", func() {
			for _, tc := range []struct {
				name       string
				code       string
				constraint string
			}{
				{name: "serialization failures", code: "40001"},
				{name: "deadlocks", code: "40P01"},
				{name: "entry sequence conflicts", code: "23505", constraint: "unique_entry_sequence_number"},
			} {
				It("should retry "+tc.name+" and eventually succeed", func() {
					// Arrange.
					attempts := 0
					conflict := &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint}

					// Act.
					err := uow.RetryTransactionWithPolicy(context.Background(), noDelayRetryPolicy(3), func(context.Context) error {
						attempts++
						if attempts < 3 {
							return fmt.Errorf("save ledger entry: %w", conflict)
						}
						return nil
					})

					// Assert.
					Expect(err).NotTo(HaveOccurred())
					Expect(attempts).To(Equal(3))
				})
			}
		})

		Context("error cases", func() {
			for _, tc := range []struct {
				name       string
				code       string
				constraint string
			}{
				{name: "serialization failures", code: "40001"},
				{name: "deadlocks", code: "40P01"},
				{name: "entry sequence conflicts", code: "23505", constraint: "unique_entry_sequence_number"},
			} {
				It("should stop after the retry limit for "+tc.name, func() {
					// Arrange.
					attempts := 0
					conflict := &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint}

					// Act.
					err := uow.RetryTransactionWithPolicy(context.Background(), noDelayRetryPolicy(3), func(context.Context) error {
						attempts++
						return conflict
					})

					// Assert.
					Expect(attempts).To(Equal(3))
					Expect(errors.Is(err, uow.ErrMaxRetriesExceeded)).To(BeTrue())
					Expect(errors.Is(err, conflict)).To(BeTrue())
				})
			}

			It("should not retry idempotency key uniqueness violations", func() {
				// Arrange.
				attempts := 0
				violation := &pgconn.PgError{Code: "23505", ConstraintName: "idx_transactions_idempotency_key"}

				// Act.
				err := uow.RetryTransactionWithPolicy(context.Background(), noDelayRetryPolicy(5), func(context.Context) error {
					attempts++
					return violation
				})

				// Assert.
				Expect(attempts).To(Equal(1))
				Expect(errors.Is(err, violation)).To(BeTrue())
				Expect(errors.Is(err, uow.ErrMaxRetriesExceeded)).To(BeFalse())
			})

			It("should stop retries when the context is canceled during backoff", func() {
				// Arrange.
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				attempts := 0
				policy := noDelayRetryPolicy(5)
				policy.Wait = func(ctx context.Context, _ time.Duration) error {
					return ctx.Err()
				}

				// Act.
				err := uow.RetryTransactionWithPolicy(ctx, policy, func(context.Context) error {
					attempts++
					cancel()
					return &pgconn.PgError{Code: "40001"}
				})

				// Assert.
				Expect(errors.Is(err, context.Canceled)).To(BeTrue())
				Expect(attempts).To(Equal(1))
			})
		})
	})
})
