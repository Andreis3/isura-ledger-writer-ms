//go:build integration

package postgres_test

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// acknowledgedCommitLostTx commits on the actual PostgreSQL server and then
// drops the success acknowledgement. This deterministic transport fault
// injection tests the application's uncertain-commit recovery path; it does
// not claim to emulate the exact TCP packet timing.
type acknowledgedCommitLostTx struct{ pgx.Tx }

func (t *acknowledgedCommitLostTx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	return errors.New("injected lost COMMIT acknowledgement")
}

var _ = Describe("CHAOS :: COMMIT ACK LOSS :: APPLICATION RECOVERY", func() {
	It("recovers the committed transfer and outbox without duplicate entries", func() {
		debit, credit := insertAccountsForCommand(ctx, pool)
		key := "ack-" + uuid.NewString()
		amount := int64(10)
		currency := string(money.BRL)
		operation := string(transaction.OperationTransfer)
		input := dto.CreateTransactionInput{
			IdempotencyKey: &key, DebitAccountID: &debit, CreditAccountID: &credit,
			Amount: &amount, Currency: &currency, Operation: &operation,
		}
		injected := false
		faultyUOW := uow.NewUnitOfWorkWithBegin(func(ctx context.Context) (pgx.Tx, error) {
			realTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
			if err != nil {
				return nil, err
			}
			if !injected {
				injected = true
				return &acknowledgedCommitLostTx{Tx: realTx}, nil
			}
			return realTx, nil
		})
		sut := command.NewCreateTransaction(
			faultyUOW,
			repository.NewAccountRepository(pool),
			repository.NewTransactionRepository(pool),
			repository.NewOutBoxRepository(pool),
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{},
			application.DefaultMaxTransactionEntries,
		)
		first, err := sut.Execute(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(injected).To(BeTrue())
		Expect(first).NotTo(BeNil())
		Expect(first.IdempotentReplay).To(BeTrue(), "the committed result should be recovered by idempotency key")

		replay, err := newIntegrationCreateTransaction(pool).Execute(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay.IdempotentReplay).To(BeTrue())
		Expect(replay.TransactionID).To(Equal(first.TransactionID))

		var transactions, entries, events int
		Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE idempotency_key=$1", key).Scan(&transactions)).To(Succeed())
		Expect(pool.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id=$1", *first.TransactionID).Scan(&entries)).To(Succeed())
		Expect(pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE aggregate_id=$1", *first.TransactionID).Scan(&events)).To(Succeed())
		Expect(transactions).To(Equal(1))
		Expect(entries).To(Equal(2))
		Expect(events).To(Equal(1))

		DeferCleanup(func() {
			_, cleanupErr := pool.Exec(ctx, "DELETE FROM outbox_events WHERE aggregate_id=$1", *first.TransactionID)
			Expect(cleanupErr).NotTo(HaveOccurred())
			_, cleanupErr = pool.Exec(ctx, "DELETE FROM entries WHERE transaction_id=$1", *first.TransactionID)
			Expect(cleanupErr).NotTo(HaveOccurred())
			_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id=$1", *first.TransactionID)
			Expect(cleanupErr).NotTo(HaveOccurred())
		})
	})
})
