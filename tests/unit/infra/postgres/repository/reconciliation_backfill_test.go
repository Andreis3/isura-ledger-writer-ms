//go:build unit

package repository_test

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	repositoryfake "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type repositoryTransactionFake struct {
	pgx.Tx
	db      *repositoryfake.QuerierFake
	execErr error
	queries []string
}

func (f *repositoryTransactionFake) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.queries = append(f.queries, query)
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (f *repositoryTransactionFake) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return f.db.Query(ctx, query, args...)
}
func (f *repositoryTransactionFake) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return f.db.QueryRow(ctx, query, args...)
}
func (f *repositoryTransactionFake) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return f.db.SendBatch(ctx, batch)
}

var _ pgx.Tx = (*repositoryTransactionFake)(nil)
var _ database.Querier = (*repositoryTransactionFake)(nil)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: RECONCILIATION", func() {
	Describe("#Run", func() {
		Context("success cases", func() {
			It("should confirm a ledger whose persisted balance matches replay", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryRows: []pgx.Rows{
					&repositoryfake.RowsFake{Data: [][]any{reconciliationAccountRow()}},
					&repositoryfake.RowsFake{Data: [][]any{reconciliationEntryRow(1, 100)}},
				}}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewLedgerReconciliation(db)
				ctx := database.WithTx(context.Background(), tx)

				// Act.
				report, err := sut.Run(ctx)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(report.AccountsChecked).To(Equal(1))
				Expect(report.EntriesChecked).To(Equal(1))
				Expect(report.Reconciled).To(BeTrue())
				Expect(report.Mismatches).To(BeEmpty())
			})

			It("should report the first persisted balance that differs from replay", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryRows: []pgx.Rows{
					&repositoryfake.RowsFake{Data: [][]any{reconciliationAccountRow()}},
					&repositoryfake.RowsFake{Data: [][]any{reconciliationEntryRow(1, 90)}},
				}}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewLedgerReconciliation(db)

				// Act.
				report, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(report.Reconciled).To(BeFalse())
				Expect(report.Mismatches).To(HaveLen(1))
				Expect(report.Mismatches[0].ExpectedBalance).To(Equal(int64(100)))
				Expect(report.Mismatches[0].PersistedBalance).To(Equal(int64(90)))
			})
		})

		Context("error cases", func() {
			It("should require an active transaction", func() {
				// Arrange.
				sut := repository.NewLedgerReconciliation(&repositoryfake.QuerierFake{})

				// Act.
				_, err := sut.Run(context.Background())

				// Assert.
				Expect(err).To(MatchError(repository.ErrReconciliationTransactionRequired))
			})

			It("should return account query errors with context", func() {
				// Arrange.
				wantErr := errors.New("account query failed")
				db := &repositoryfake.QuerierFake{QueryErr: wantErr}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewLedgerReconciliation(db)

				// Act.
				_, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err.Error()).To(ContainSubstring("load accounts for reconciliation"))
				Expect(errors.Is(err, wantErr)).To(BeTrue())
			})

			It("should reject entries that reference unknown accounts", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryRows: []pgx.Rows{
					&repositoryfake.RowsFake{},
					&repositoryfake.RowsFake{Data: [][]any{reconciliationEntryRow(1, 0)}},
				}}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewLedgerReconciliation(db)

				// Act.
				_, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown account"))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: BACKFILL", func() {
	Describe("#Run", func() {
		Context("success cases", func() {
			It("should recalculate persisted balances within the caller transaction", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryRows: []pgx.Rows{
					&repositoryfake.RowsFake{Data: [][]any{reconciliationAccountRow()}},
					&repositoryfake.RowsFake{Data: [][]any{reconciliationEntryRow(1, 90)}},
				}}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewHistoricalBalanceBackfill(db)

				// Act.
				report, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(report.Accounts).To(Equal(1))
				Expect(report.Entries).To(Equal(1))
				Expect(report.PersistedBalanceMismatches).To(Equal(1))
				Expect(report.UpdatedEntries).To(Equal(1))
				Expect(report.ReadyForActivation).To(BeTrue())
				Expect(tx.queries).To(HaveLen(2))
				Expect(tx.queries[1]).To(ContainSubstring("UPDATE entries SET running_balance"))
			})
		})

		Context("error cases", func() {
			It("should require an active transaction", func() {
				// Arrange.
				sut := repository.NewHistoricalBalanceBackfill(&repositoryfake.QuerierFake{})

				// Act.
				_, err := sut.Run(context.Background())

				// Assert.
				Expect(err).To(MatchError(repository.ErrBackfillTransactionRequired))
			})

			It("should stop when the transaction cannot lock ledger tables", func() {
				// Arrange.
				wantErr := errors.New("lock failed")
				tx := &repositoryTransactionFake{db: &repositoryfake.QuerierFake{}, execErr: wantErr}
				sut := repository.NewHistoricalBalanceBackfill(tx)

				// Act.
				_, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err.Error()).To(ContainSubstring("lock ledger tables"))
				Expect(errors.Is(err, wantErr)).To(BeTrue())
			})

			It("should reject an entry that references an unknown account", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryRows: []pgx.Rows{
					&repositoryfake.RowsFake{},
					&repositoryfake.RowsFake{Data: [][]any{reconciliationEntryRow(1, 0)}},
				}}
				tx := &repositoryTransactionFake{db: db}
				sut := repository.NewHistoricalBalanceBackfill(db)

				// Act.
				_, err := sut.Run(database.WithTx(context.Background(), tx))

				// Assert.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("references unknown account"))
			})
		})
	})
})

func reconciliationAccountRow() []any {
	return []any{
		pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
		pgtype.Text{String: string(account.Asset), Valid: true},
		pgtype.Text{String: string(account.BalanceUnrestricted), Valid: true},
		pgtype.Text{String: string(money.BRL), Valid: true},
	}
}

func reconciliationEntryRow(sequence, persisted int64) []any {
	return []any{
		pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
		pgtype.Int8{Int64: sequence, Valid: true},
		pgtype.Text{String: string(transaction.Debit), Valid: true},
		pgtype.Int8{Int64: 100, Valid: true},
		pgtype.Text{String: string(money.BRL), Valid: true},
		pgtype.Int8{Int64: persisted, Valid: true},
	}
}
