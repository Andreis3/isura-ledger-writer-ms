//go:build integration

package postgres_test

import (
	"fmt"
	database "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	repository "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	uuid "github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: HISTORICAL BALANCE BACKFILL", func() {
	Context("database behavior", func() {
		It("should recalculate historical balances using account nature and preserve sequences", func() {
			// Normalize existing ledger data so report assertions cover only this fixture.
			_, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
			Expect(err).NotTo(HaveOccurred())

			accountID := newIDV7()
			transactionID := newIDV7()
			now := time.Now()
			_, err = tx.Exec(ctx, `
					INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at)
					VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_NON_NEGATIVE', 'BRL', $5, $5)`,
				accountID, uuid.NewString(), fmt.Sprintf("%d", now.UnixNano()), "52998224725", now)
			Expect(err).NotTo(HaveOccurred())
			_, err = tx.Exec(ctx, `
					INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
					VALUES ($1, $2, $3, 'PENDING', 'TRANSFER', 100, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
			Expect(err).NotTo(HaveOccurred())
			for position, entry := range []struct {
				id, direction                    string
				sequence, amount, runningBalance int64
			}{
				{id: newIDV7(), direction: "DEBIT", sequence: 1, amount: 100, runningBalance: -100},
				{id: newIDV7(), direction: "CREDIT", sequence: 2, amount: 40, runningBalance: -60},
			} {
				_, err = tx.Exec(ctx, `
						INSERT INTO entries (id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, created_at)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'BRL', $9)`,
					entry.id, accountID, transactionID, entry.sequence, position, entry.direction, entry.amount, entry.runningBalance, now)
				Expect(err).NotTo(HaveOccurred())
			}

			report, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
			Expect(err).NotTo(HaveOccurred())
			Expect(report.UpdatedEntries).To(Equal(2))
			Expect(report.PersistedBalanceMismatches).To(Equal(2))

			repeatedReport, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
			Expect(err).NotTo(HaveOccurred())
			Expect(repeatedReport.UpdatedEntries).To(Equal(0))

			rows, err := tx.Query(ctx, `SELECT sequence_number, running_balance FROM entries WHERE account_id = $1 ORDER BY sequence_number`, accountID)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()
			type persistedEntry struct {
				sequence int64
				balance  int64
			}
			var entries []persistedEntry
			for rows.Next() {
				var sequence, balance int64
				Expect(rows.Scan(&sequence, &balance)).To(Succeed())
				entries = append(entries, persistedEntry{sequence: sequence, balance: balance})
			}
			Expect(rows.Err()).NotTo(HaveOccurred())
			Expect(entries).To(Equal([]persistedEntry{{sequence: 1, balance: 100}, {sequence: 2, balance: 60}}))
		})
	})
})
