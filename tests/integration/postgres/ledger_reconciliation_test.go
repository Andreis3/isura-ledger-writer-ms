//go:build integration

package postgres_test

import (
	"errors"
	database "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	repository "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	uuid "github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: LEDGER RECONCILIATION", func() {
	Context("database behavior", func() {
		It("should keep reconciliation on one snapshot while a balance is updated concurrently", func() {
			var existingEntries int
			Expect(pool.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&existingEntries)).To(Succeed())
			accountID := insertAccount(ctx, pool)
			transactionID := newIDV7()
			entryID := newIDV7()
			now := time.Now()
			_, err := pool.Exec(ctx, `
					INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
					VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 50, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", now)
			Expect(err).NotTo(HaveOccurred())
			_, err = pool.Exec(ctx, `
					INSERT INTO entries (id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, created_at)
					VALUES ($1, $2, $3, 1, 0, 'DEBIT', 50, 50, 'BRL', $4)`, entryID, accountID, transactionID, now)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
				Expect(cleanupErr).NotTo(HaveOccurred())
			})

			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			})
			var snapshotBalance int64
			Expect(auditTx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE id = $1", entryID).Scan(&snapshotBalance)).To(Succeed())
			Expect(snapshotBalance).To(Equal(int64(50)))
			_, err = pool.Exec(ctx, "UPDATE entries SET running_balance = 51 WHERE id = $1", entryID)
			Expect(err).NotTo(HaveOccurred())

			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())
			Expect(report.EntriesChecked).To(Equal(existingEntries + 1))
			_, found := reconciliationMismatchFor(report, accountID)
			Expect(found).To(BeFalse())
			Expect(auditTx.Commit(ctx)).To(Succeed())
		})
		It("should audit persisted entries without changing ledger records", func() {
			accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 125)
			DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

			before := reconciliationSnapshot(ctx, pool, accountID)
			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			})
			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())
			Expect(auditTx.Rollback(ctx)).To(Succeed())

			Expect(report.AccountsChecked).To(BeNumerically(">=", 1))
			Expect(report.EntriesChecked).To(BeNumerically(">=", 1))
			Expect(report.Mismatches).NotTo(ContainElement(HaveField("AccountID", accountID)))
			Expect(reconciliationSnapshot(ctx, pool, accountID)).To(Equal(before))
		})
		It("should report the replayed and persisted balances for a PostgreSQL mismatch", func() {
			accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 120)
			DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			}()
			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())

			mismatch, found := reconciliationMismatchFor(report, accountID)
			Expect(found).To(BeTrue())
			Expect(mismatch.EntriesChecked).To(Equal(1))
			Expect(mismatch.ExpectedBalance).To(Equal(int64(125)))
			Expect(mismatch.PersistedBalance).To(Equal(int64(120)))
			Expect(mismatch.Currency).To(Equal("BRL"))
		})
		It("should return an explicit error for an entry referencing an unknown account", func() {
			transactionID := newIDV7()
			entryID := newIDV7()
			missingAccountID := newIDV7()
			now := time.Now()
			_, err := pool.Exec(ctx, `
					INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
					VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 75, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", now)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
				Expect(cleanupErr).NotTo(HaveOccurred())
			})
			fixtureTx, err := pool.Begin(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = fixtureTx.Exec(ctx, "SET LOCAL session_replication_role = replica")
			Expect(err).NotTo(HaveOccurred())
			_, err = fixtureTx.Exec(ctx, `
					INSERT INTO entries (id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, created_at)
					VALUES ($1, $2, $3, 1, 0, 'DEBIT', 75, 75, 'BRL', $4)`, entryID, missingAccountID, transactionID, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(fixtureTx.Commit(ctx)).To(Succeed())

			before := reconciliationEntrySnapshot(ctx, pool, entryID)
			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			})
			_, err = repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown account"))
			Expect(auditTx.Rollback(ctx)).To(Succeed())
			Expect(reconciliationEntrySnapshot(ctx, pool, entryID)).To(Equal(before))
		})
	})
})
