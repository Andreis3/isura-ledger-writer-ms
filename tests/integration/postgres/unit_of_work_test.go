//go:build integration

package postgres_test

import (
	"context"
	"errors"
	database "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	repository "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	uow "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: UNIT OF WORK", func() {
	Context("database behavior", func() {
		It("should persist a balanced transaction and its outbox atomically", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			entityTransaction := newTransaction(accountA, accountB)
			transactionRepo := repository.NewTransactionRepository(pool)
			outboxRepo := repository.NewOutBoxRepository(pool)
			txContext := database.WithTx(ctx, tx)

			Expect(prepareTransactionLedger(ctx, txContext, entityTransaction, pool)).To(Succeed())
			Expect(transactionRepo.Save(txContext, entityTransaction)).To(Succeed())
			Expect(outboxRepo.Save(txContext, newOutbox(entityTransaction.ID.String()))).To(Succeed())

			var transactions, entries, outboxes int
			transactionID := entityTransaction.ID.String()
			Expect(tx.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", transactionID).Scan(&transactions)).To(Succeed())
			Expect(tx.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1", transactionID).Scan(&entries)).To(Succeed())
			Expect(tx.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE aggregate_id = $1", transactionID).Scan(&outboxes)).To(Succeed())
			Expect(transactions).To(Equal(1))
			Expect(entries).To(Equal(2))
			Expect(outboxes).To(Equal(1))
		})
		It("should commit or roll back transaction, entries, and outbox as one unit", func() {
			accountA := insertAccount(ctx, pool)
			accountB := insertAccount(ctx, pool)
			transactionRepo := repository.NewTransactionRepository(pool)
			outboxRepo := repository.NewOutBoxRepository(pool)
			unitOfWork := uow.NewUnitOfWork(pool)

			committed := newTransaction(accountA, accountB)
			err := unitOfWork.WithTransaction(ctx, func(txCtx context.Context) error {
				if err := prepareTransactionLedger(ctx, txCtx, committed, pool); err != nil {
					return err
				}
				if err := transactionRepo.Save(txCtx, committed); err != nil {
					return err
				}
				return outboxRepo.Save(txCtx, newOutbox(committed.ID.String()))
			})
			Expect(err).NotTo(HaveOccurred())

			assertLedgerRecords(ctx, pool, committed.ID.String(), 1, 2, 1)

			rolledBack := newTransaction(accountA, accountB)
			expectedFailure := errors.New("force atomic rollback")
			err = unitOfWork.WithTransaction(ctx, func(txCtx context.Context) error {
				if err := prepareTransactionLedger(ctx, txCtx, rolledBack, pool); err != nil {
					return err
				}
				if err := transactionRepo.Save(txCtx, rolledBack); err != nil {
					return err
				}
				if err := outboxRepo.Save(txCtx, newOutbox(rolledBack.ID.String())); err != nil {
					return err
				}
				return expectedFailure
			})
			Expect(err).To(MatchError(expectedFailure))

			assertLedgerRecords(ctx, pool, rolledBack.ID.String(), 0, 0, 0)
		})
		It("should roll back four postings and the outbox event after a downstream failure", func() {
			// Arrange: accounts are committed so the UoW can read them independently.
			accountA, accountB := insertCommittedAccounts(ctx, pool)
			entityTransaction := newMultiEntryTransaction(accountA, accountB)
			defer cleanupCommittedAccounts(ctx, pool, []string{accountA, accountB}, []string{entityTransaction.ID.String()})
			transactionRepo := repository.NewTransactionRepository(pool)
			outboxRepo := repository.NewOutBoxRepository(pool)
			failure := errors.New("simulate downstream failure after outbox write")

			// Act: persist transaction, four postings and the outbox event, then fail.
			err := uow.NewUnitOfWork(pool).WithTransaction(ctx, func(txCtx context.Context) error {
				if err := prepareTransactionLedger(ctx, txCtx, entityTransaction, pool); err != nil {
					return err
				}
				if err := transactionRepo.Save(txCtx, entityTransaction); err != nil {
					return err
				}
				// Ensure the postings exist inside the active database transaction.
				transactionTx, ok := database.ExtractTx(txCtx)
				if !ok {
					return errors.New("missing active unit of work transaction")
				}
				var count int
				if err := transactionTx.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1", entityTransaction.ID.String()).Scan(&count); err != nil {
					return err
				}
				if count != 4 {
					return errors.New("expected four entries before outbox failure")
				}
				if err := outboxRepo.Save(txCtx, newOutbox(entityTransaction.ID.String())); err != nil {
					return err
				}
				return failure
			})

			// Assert: no transaction, posting, or event leaks past rollback.
			Expect(errors.Is(err, failure)).To(BeTrue())
			assertLedgerRecords(ctx, pool, entityTransaction.ID.String(), 0, 0, 0)
			for _, accountID := range []string{accountA, accountB} {
				var count int
				Expect(pool.QueryRow(ctx, "SELECT count(*) FROM entries WHERE account_id = $1", accountID).Scan(&count)).To(Succeed())
				Expect(count).To(BeZero())
			}
		})

		It("should roll back transaction, entries and outbox together", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			entityTransaction := newTransaction(accountA, accountB)
			txContext := database.WithTx(ctx, tx)
			Expect(prepareTransactionLedger(ctx, txContext, entityTransaction, pool)).To(Succeed())
			Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(Succeed())
			Expect(repository.NewOutBoxRepository(pool).Save(txContext, newOutbox(entityTransaction.ID.String()))).To(Succeed())
			Expect(tx.Rollback(ctx)).To(Succeed())

			var count int
			Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", entityTransaction.ID.String()).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(0))
			tx = nopTx{}
		})
	})
})
