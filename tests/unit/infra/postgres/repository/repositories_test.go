//go:build unit

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	repositoryfake "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRepositories(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Repository Adapter Suite")
}

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: TRANSACTION", func() {
	Describe("#FindLatestLedgerState", func() {
		Context("success cases", func() {
			It("should return an empty state when the account has no entries", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{QueryErr: pgx.ErrNoRows}
				sut := repository.NewTransactionRepository(db)

				// Act.
				sequence, balance, err := sut.FindLatestLedgerState(context.Background(), "account")

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(sequence).To(BeZero())
				Expect(balance).To(BeZero())
			})

			It("should return the latest persisted sequence and balance", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{Row: repositoryfake.RowFake{Values: []any{int64(8), int64(1250)}}}
				sut := repository.NewTransactionRepository(db)

				// Act.
				sequence, balance, err := sut.FindLatestLedgerState(context.Background(), "account")

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(sequence).To(Equal(int64(8)))
				Expect(balance).To(Equal(int64(1250)))
			})
		})

		Context("error cases", func() {
			It("should wrap database errors", func() {
				// Arrange.
				wantErr := errors.New("database unavailable")
				sut := repository.NewTransactionRepository(&repositoryfake.QuerierFake{QueryErr: wantErr})

				// Act.
				_, _, err := sut.FindLatestLedgerState(context.Background(), "account")

				// Assert.
				Expect(err.Error()).To(ContainSubstring("find latest ledger state"))
				Expect(errors.Is(err, wantErr)).To(BeTrue())
			})
		})
	})

	Describe("#ExistsByIdempotencyKey", func() {
		Context("success cases", func() {
			It("should return the stored existence value", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{Row: repositoryfake.RowFake{Values: []any{true}}}
				sut := repository.NewTransactionRepository(db)

				// Act.
				exists, err := sut.ExistsByIdempotencyKey(context.Background(), "key")

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})
		Context("error cases", func() {
			It("should propagate existence query failures", func() {
				// Arrange.
				wantErr := errors.New("existence query failed")
				sut := repository.NewTransactionRepository(&repositoryfake.QuerierFake{QueryErr: wantErr})

				// Act.
				_, err := sut.ExistsByIdempotencyKey(context.Background(), "key")

				// Assert.
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#Find", func() {
		Context("success cases", func() {
			It("should restore a transaction and its entries from database rows", func() {
				// Arrange.
				created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				db := &repositoryfake.QuerierFake{
					Row: repositoryfake.RowFake{Values: []any{
						pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
						pgtype.Text{String: "key", Valid: true},
						pgtype.Text{String: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Valid: true},
						pgtype.Text{String: string(transaction.Completed), Valid: true},
						pgtype.Int8{Int64: 100, Valid: true},
						pgtype.Text{String: string(transaction.OperationDeposit), Valid: true},
						pgtype.Text{String: string(money.BRL), Valid: true},
						[]byte(`{"source":"test"}`),
						pgtype.Timestamptz{Time: created, Valid: true},
						pgtype.Timestamptz{Time: created.Add(time.Minute), Valid: true},
					}},
					QueryRows: []pgx.Rows{
						&repositoryfake.RowsFake{Data: [][]any{
							persistedEntry("019ff448-c43d-70d3-83c7-dfa067446b02", "d589965c-1622-4329-98f9-f13354a2e4dc", "DEBIT", created, 1, 0),
							persistedEntry("019ff448-c43d-70d3-83c7-dfa067446b03", "d589965c-1622-4329-98f9-f13354a2e4de", "CREDIT", created, 2, 1),
						}},
						&repositoryfake.RowsFake{Data: [][]any{
							{pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b04", Valid: true}, pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true}, pgtype.Text{String: "TRANSFER", Valid: true}, pgtype.Text{String: "main transfer", Valid: true}, pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b02", Valid: true}},
							{pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b04", Valid: true}, pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true}, pgtype.Text{String: "TRANSFER", Valid: true}, pgtype.Text{String: "main transfer", Valid: true}, pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b03", Valid: true}},
						}},
					},
				}
				sut := repository.NewTransactionRepository(db)

				// Act.
				found, err := sut.Find(context.Background(), transaction.TransactionCriteria{WithEntries: true})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(found.ID.String()).To(Equal("019ff448-c43d-70d3-83c7-dfa067446b01"))
				Expect(found.Entries).To(HaveLen(2))
				Expect(found.Entries[0].Direction).To(Equal(transaction.Debit))
				Expect(found.Entries[1].Direction).To(Equal(transaction.Credit))
				Expect(found.Metadata).To(Equal(map[string]string{"source": "test"}))
				Expect(found.Movements).To(HaveLen(1))
				Expect(found.Movements[0].Description).To(Equal("main transfer"))
				Expect(found.Movements[0].Entries).To(HaveLen(2))
			})
		})

		Context("error cases", func() {
			It("should return the domain not found error for a missing transaction", func() {
				// Arrange.
				sut := repository.NewTransactionRepository(&repositoryfake.QuerierFake{QueryErr: pgx.ErrNoRows})

				// Act.
				found, err := sut.Find(context.Background(), transaction.TransactionCriteria{})

				// Assert.
				Expect(found).To(BeNil())
				Expect(err).To(MatchError(transaction.ErrTransactionNotFound))
			})

			It("should propagate query failures", func() {
				// Arrange.
				wantErr := errors.New("query failed")
				sut := repository.NewTransactionRepository(&repositoryfake.QuerierFake{QueryErr: wantErr})

				// Act.
				_, err := sut.Find(context.Background(), transaction.TransactionCriteria{})

				// Assert.
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#Save", func() {
		Context("success cases", func() {
			It("should persist a transaction through one batch command", func() {
				// Arrange.
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entity, err := transaction.NewTransactionBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithIdempotencyKey("save-test").
					WithFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
					WithStatus(transaction.Completed).
					WithOperation(transaction.OperationDeposit).
					WithAmount(amount).
					Build()
				Expect(err).NotTo(HaveOccurred())
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewTransactionRepository(db)

				// Act.
				err = sut.Save(context.Background(), entity)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(db.ExecCalls).To(Equal(1))
			})
		})

		Context("error cases", func() {
			It("should stop and return the first batch execution error", func() {
				// Arrange.
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entity, err := transaction.NewTransactionBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithIdempotencyKey("save-test").
					WithFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
					WithStatus(transaction.Completed).
					WithOperation(transaction.OperationDeposit).
					WithAmount(amount).
					Build()
				Expect(err).NotTo(HaveOccurred())
				wantErr := errors.New("batch failed")
				batch := &repositoryfake.BatchResultsFake{Errors: []error{wantErr}}
				sut := repository.NewTransactionRepository(&repositoryfake.QuerierFake{Batch: batch})

				// Act.
				err = sut.Save(context.Background(), entity)

				// Assert.
				Expect(err).To(MatchError(wantErr))
				Expect(batch.Closed).To(BeTrue())
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: ACCOUNT", func() {
	Describe("#Save", func() {
		Context("success cases", func() {
			It("should persist an account", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewAccountRepository(db)

				// Act.
				err := sut.Save(context.Background(), validAccount())

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(db.Queries[0]).To(ContainSubstring("INSERT INTO"))
				Expect(db.Arguments[0]).To(HaveLen(10))
			})
		})

		Context("error cases", func() {
			It("should translate a duplicate key into an already exists error", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{ExecErr: &pgconn.PgError{Code: "23505"}}
				sut := repository.NewAccountRepository(db)

				// Act.
				err := sut.Save(context.Background(), validAccount())

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeAlreadyExists))
			})

			It("should translate other storage failures into a database error", func() {
				// Arrange.
				sut := repository.NewAccountRepository(&repositoryfake.QuerierFake{ExecErr: errors.New("write failed")})

				// Act.
				err := sut.Save(context.Background(), validAccount())

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeDatabaseError))
			})
		})
	})

	Describe("#FindAccount", func() {
		Context("success cases", func() {
			It("should restore account details from the persisted row", func() {
				// Arrange.
				created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				db := &repositoryfake.QuerierFake{Row: repositoryfake.RowFake{Values: []any{
					pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa0674469b7", Valid: true},
					pgtype.Text{String: "d589965c-1622-4329-98f9-f13354a2e4dc", Valid: true},
					pgtype.Text{String: "123456", Valid: true},
					pgtype.Text{String: "529.982.247-25", Valid: true},
					pgtype.Text{String: string(account.StatusActive), Valid: true},
					pgtype.Text{String: string(account.Asset), Valid: true},
					pgtype.Text{String: string(account.BalanceUnrestricted), Valid: true},
					pgtype.Text{String: string(money.BRL), Valid: true},
					pgtype.Timestamptz{Time: created, Valid: true},
					pgtype.Timestamptz{Time: created.Add(time.Hour), Valid: true},
				}}}
				sut := repository.NewAccountRepository(db)

				// Act.
				found, err := sut.FindAccount(context.Background(), criteria.AccountCriteria{ID: stringPointer("019ff448-c43d-70d3-83c7-dfa0674469b7")})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(found.ID.String()).To(Equal("019ff448-c43d-70d3-83c7-dfa0674469b7"))
				Expect(found.AccountExternalID).To(Equal("d589965c-1622-4329-98f9-f13354a2e4dc"))
				Expect(found.BalancePolicy).To(Equal(account.BalanceUnrestricted))
			})
		})

		Context("error cases", func() {
			It("should return an empty result when the account does not exist", func() {
				// Arrange.
				sut := repository.NewAccountRepository(&repositoryfake.QuerierFake{QueryErr: pgx.ErrNoRows})

				// Act.
				found, err := sut.FindAccount(context.Background(), criteria.AccountCriteria{})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeNil())
			})

			It("should translate database failures", func() {
				// Arrange.
				sut := repository.NewAccountRepository(&repositoryfake.QuerierFake{QueryErr: errors.New("read failed")})

				// Act.
				_, err := sut.FindAccount(context.Background(), criteria.AccountCriteria{})

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeDatabaseError))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: BALANCE", func() {
	Describe("#Save", func() {
		Context("success cases", func() {
			It("should persist a balance", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewBalanceRepository(db)

				// Act.
				err := sut.Save(context.Background(), validBalance())

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(db.Arguments[0]).To(HaveLen(6))
			})
		})

		Context("error cases", func() {
			It("should report a duplicate balance", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{ExecErr: &pgconn.PgError{Code: "23505"}}
				sut := repository.NewBalanceRepository(db)

				// Act.
				err := sut.Save(context.Background(), validBalance())

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeAlreadyExists))
			})

			It("should translate other storage failures into a persistence error", func() {
				// Arrange.
				sut := repository.NewBalanceRepository(&repositoryfake.QuerierFake{ExecErr: errors.New("write failed")})

				// Act.
				err := sut.Save(context.Background(), validBalance())

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeDatabaseError))
			})
		})
	})

	Describe("#Find", func() {
		Context("success cases", func() {
			It("should restore a persisted balance", func() {
				// Arrange.
				created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				db := &repositoryfake.QuerierFake{Row: repositoryfake.RowFake{Values: []any{
					pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa0674469b7", Valid: true},
					pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa0674469b8", Valid: true},
					pgtype.Int8{Int64: 500, Valid: true},
					pgtype.Text{String: string(money.BRL), Valid: true},
					pgtype.Timestamptz{Time: created, Valid: true},
					pgtype.Timestamptz{Time: created.Add(time.Hour), Valid: true},
				}}}
				sut := repository.NewBalanceRepository(db)

				// Act.
				found, err := sut.Find(context.Background(), criteria.BalanceCriteria{AccountID: stringPointer("019ff448-c43d-70d3-83c7-dfa0674469b8")})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(found.ID().String()).To(Equal("019ff448-c43d-70d3-83c7-dfa0674469b7"))
				Expect(found.AccountID()).To(Equal("019ff448-c43d-70d3-83c7-dfa0674469b8"))
				Expect(found.Amount().Amount()).To(Equal(int64(500)))
			})
		})

		Context("error cases", func() {
			It("should return no balance when the account has no balance row", func() {
				// Arrange.
				sut := repository.NewBalanceRepository(&repositoryfake.QuerierFake{QueryErr: pgx.ErrNoRows})

				// Act.
				found, err := sut.Find(context.Background(), criteria.BalanceCriteria{})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeNil())
			})

			It("should translate balance query failures", func() {
				// Arrange.
				sut := repository.NewBalanceRepository(&repositoryfake.QuerierFake{QueryErr: errors.New("read failed")})

				// Act.
				_, err := sut.Find(context.Background(), criteria.BalanceCriteria{})

				// Assert.
				var domainErr *fault.DomainError
				Expect(errors.As(err, &domainErr)).To(BeTrue())
				Expect(domainErr.Code).To(Equal(fault.CodeDatabaseError))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: OUTBOX", func() {
	Describe("#Save", func() {
		Context("success cases", func() {
			It("should persist an outbox event", func() {
				// Arrange.
				item, err := outbox.NewOutbox("019ff448-c43d-70d3-83c7-dfa067446b01", []byte(`{"ok":true}`))
				Expect(err).NotTo(HaveOccurred())
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				err = sut.Save(context.Background(), item)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(db.Queries).To(HaveLen(1))
				Expect(db.Arguments[0]).To(HaveLen(10))
			})
		})

		Context("error cases", func() {
			It("should return a persistence error", func() {
				// Arrange.
				wantErr := errors.New("insert failed")
				item, err := outbox.NewOutbox("019ff448-c43d-70d3-83c7-dfa067446b01", []byte(`{}`))
				Expect(err).NotTo(HaveOccurred())
				sut := repository.NewOutBoxRepository(&repositoryfake.QuerierFake{ExecErr: wantErr})

				// Act.
				err = sut.Save(context.Background(), item)

				// Assert.
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#ClaimPending", func() {
		Context("success cases", func() {
			It("should return no rows without querying when the claim limits are invalid", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				claimed, err := sut.ClaimPending(context.Background(), 0, 3, time.Minute)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(claimed).To(BeNil())
				Expect(db.Queries).To(BeEmpty())
			})

			It("should return an empty batch when no events are pending", func() {
				// Arrange.
				sut := repository.NewOutBoxRepository(&repositoryfake.QuerierFake{})

				// Act.
				claimed, err := sut.ClaimPending(context.Background(), 5, 3, time.Minute)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(claimed).To(BeEmpty())
			})

			It("should restore claimed events from returned rows", func() {
				// Arrange.
				created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				attempted := created.Add(time.Minute)
				rows := &repositoryfake.RowsFake{Data: [][]any{{
					pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
					pgtype.Text{String: "aggregate", Valid: true},
					pgtype.Text{String: string(outbox.Transaction), Valid: true},
					pgtype.Text{String: string(outbox.TransactionCreated), Valid: true},
					[]byte(`{"event":"created"}`),
					pgtype.Text{String: string(outbox.Failed), Valid: true},
					pgtype.Int2{Int16: 1, Valid: true},
					pgtype.Timestamptz{Time: attempted, Valid: true},
					pgtype.Timestamptz{Time: created, Valid: true},
					pgtype.Timestamptz{Valid: false},
				}}}
				db := &repositoryfake.QuerierFake{Rows: rows}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				claimed, err := sut.ClaimPending(context.Background(), 5, 3, time.Minute)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(claimed).To(HaveLen(1))
				Expect(claimed[0].ID.String()).To(Equal("019ff448-c43d-70d3-83c7-dfa067446b01"))
				Expect(claimed[0].Status).To(Equal(outbox.Failed))
				Expect(claimed[0].LastAttemptAt).To(Equal(&attempted))
				Expect(rows.Closed).To(BeTrue())
			})
		})

		Context("error cases", func() {
			It("should propagate query errors", func() {
				// Arrange.
				wantErr := errors.New("claim failed")
				sut := repository.NewOutBoxRepository(&repositoryfake.QuerierFake{QueryErr: wantErr})

				// Act.
				claimed, err := sut.ClaimPending(context.Background(), 5, 3, time.Minute)

				// Assert.
				Expect(claimed).To(BeNil())
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#FindAll", func() {
		Context("success cases", func() {
			It("should return an empty list when no events match the status", func() {
				// Arrange.
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				items, err := sut.FindAll(context.Background(), outbox.Pending, 10)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(items).To(BeNil())
				Expect(db.Arguments[0]).To(Equal([]any{outbox.Pending, 10}))
			})
		})
		Context("error cases", func() {
			It("should return row iteration failures", func() {
				// Arrange.
				wantErr := errors.New("row iteration failed")
				db := &repositoryfake.QuerierFake{Rows: &repositoryfake.RowsFake{RowsErr: wantErr}}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				items, err := sut.FindAll(context.Background(), outbox.Pending, 10)

				// Assert.
				Expect(items).To(BeNil())
				Expect(err).To(MatchError(wantErr))
			})

			It("should propagate outbox query errors", func() {
				// Arrange.
				wantErr := errors.New("query failed")
				sut := repository.NewOutBoxRepository(&repositoryfake.QuerierFake{QueryErr: wantErr})

				// Act.
				_, err := sut.FindAll(context.Background(), outbox.Pending, 10)

				// Assert.
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#UpdateOutboxData", func() {
		Context("success cases", func() {
			It("should update the persisted state and timestamps", func() {
				// Arrange.
				id, err := entity.NewID("019ff448-c43d-70d3-83c7-dfa067446b01")
				Expect(err).NotTo(HaveOccurred())
				db := &repositoryfake.QuerierFake{}
				sut := repository.NewOutBoxRepository(db)

				// Act.
				err = sut.UpdateOutboxData(context.Background(), id, outbox.UpdateOutboxData{Status: outbox.Failed, Attempts: 1})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(db.Arguments[0]).To(HaveLen(5))
				Expect(db.Arguments[0][0]).To(Equal(pgtype.Text{String: string(outbox.Failed), Valid: true}))
			})
		})
	})
})

func validAccount() *account.Account {
	entity, err := account.NewAccountBuilder().
		WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
		WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
		WithAccountNumber("123456").
		WithTaxID("529.982.247-25").
		WithType(string(account.Asset)).
		WithBalancePolicy(string(account.BalanceUnrestricted)).
		WithCurrency("BRL").
		Build()
	if err != nil {
		Fail(err.Error())
	}
	return entity
}

func validBalance() *balance.Balance {
	entity, err := balance.NewBalanceBuilder().
		WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
		WithAccountID("019ff448-c43d-70d3-83c7-dfa0674469b8").
		WithAmount(100, money.BRL).
		Build()
	if err != nil {
		Fail(err.Error())
	}
	return entity
}

func stringPointer(value string) *string { return &value }

func persistedEntry(id, accountID, direction string, created time.Time, sequence, position int64) []any {
	return []any{
		pgtype.Text{String: id, Valid: true},
		pgtype.Text{String: accountID, Valid: true},
		pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
		pgtype.Int8{Int64: sequence, Valid: true},
		pgtype.Int8{Int64: position, Valid: true},
		pgtype.Text{String: direction, Valid: true},
		pgtype.Int8{Int64: 100, Valid: true},
		pgtype.Int8{Int64: 100, Valid: true},
		pgtype.Text{String: string(money.BRL), Valid: true},
		[]byte(nil),
		pgtype.Timestamptz{Time: created, Valid: true},
	}
}
