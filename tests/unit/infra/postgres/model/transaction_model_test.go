//go:build unit

package model_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: TRANSACTION", func() {
	Describe("#ToTransactionModel", func() {
		Context("success cases", func() {
			It("should preserve legacy amount and currency for a transaction", func() {
				// Arrange.
				amount, err := money.NewMoney(250, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				tx, err := transaction.NewTransactionBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithIdempotencyKey("single-currency").
					WithFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
					WithStatus(transaction.Completed).WithOperation(transaction.OperationDeposit).
					WithAmount(amount).Build()
				Expect(err).NotTo(HaveOccurred())

				// Act.
				persisted, err := model.ToTransactionModel(tx)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.Amount).To(Equal(pgtype.Int8{Int64: 250, Valid: true}))
				Expect(persisted.Currency).To(Equal(pgtype.Text{String: string(money.BRL), Valid: true}))
			})
		})
	})

	Describe("#ToTransactionDomain", func() {
		Context("success cases", func() {
			It("should restore the persisted amount and currency", func() {
				// Arrange.
				persisted := model.Transaction{
					ID:                 pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
					IdempotencyKey:     pgtype.Text{String: "single-currency", Valid: true},
					RequestFingerprint: pgtype.Text{String: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Valid: true},
					Status:             pgtype.Text{String: string(transaction.Completed), Valid: true},
					Operation:          pgtype.Text{String: string(transaction.OperationDeposit), Valid: true},
					Amount:             pgtype.Int8{Int64: 250, Valid: true},
					Currency:           pgtype.Text{String: string(money.BRL), Valid: true},
				}

				// Act.
				decoded, err := model.ToTransactionDomain(persisted, nil)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(decoded.Amount.Amount()).To(Equal(int64(250)))
				Expect(decoded.Amount.Currency()).To(Equal(money.BRL))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: ENTRY", func() {
	Describe("#ToEntryModel", func() {
		Context("success cases", func() {
			It("should preserve transaction position in both mapping directions", func() {
				// Arrange.
				amount, err := money.NewMoney(250, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entry, err := transaction.NewEntryBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b02").
					WithTransactionID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithDirection(transaction.Debit).WithAmount(amount).Build()
				Expect(err).NotTo(HaveOccurred())
				entry.AssignAccountID("d589965c-1622-4329-98f9-f13354a2e4dc")
				entry.SetTransactionPosition(7)

				// Act.
				persisted, err := model.ToEntryModel(entry)
				Expect(err).NotTo(HaveOccurred())
				decoded, err := model.ToEntryDomain(persisted)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.TransactionPosition.Int64).To(Equal(int64(7)))
				Expect(decoded.TransactionPosition).To(Equal(int64(7)))
			})
		})
	})
})
