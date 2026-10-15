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

			It("should preserve the absence of movements for a historical transaction", func() {
				// Arrange.
				persisted := model.Transaction{
					ID:                 pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true},
					IdempotencyKey:     pgtype.Text{String: "historical", Valid: true},
					RequestFingerprint: pgtype.Text{String: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Valid: true},
					Status:             pgtype.Text{String: string(transaction.Completed), Valid: true},
					Operation:          pgtype.Text{String: string(transaction.OperationTransfer), Valid: true},
					Amount:             pgtype.Int8{Int64: 250, Valid: true},
					Currency:           pgtype.Text{String: string(money.BRL), Valid: true},
				}
				entries := historicalEntries()

				// Act.
				decoded, err := model.ToTransactionDomain(persisted, entries)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(decoded.Movements).To(BeNil())
			})
		})
	})
})

func historicalEntries() []*transaction.Entry {
	amount, err := money.NewMoney(250, money.BRL)
	Expect(err).NotTo(HaveOccurred())
	entries := make([]*transaction.Entry, 0, 2)
	for index, direction := range []transaction.Direction{transaction.Debit, transaction.Credit} {
		entry, buildErr := transaction.NewEntryBuilder().WithID().WithTransactionID("019ff448-c43d-70d3-83c7-dfa067446b01").
			WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").WithDirection(direction).WithAmount(amount).Build()
		Expect(buildErr).NotTo(HaveOccurred())
		entry.AssignAccountID("d589965c-1622-4329-98f9-f13354a2e4dc")
		entry.SetTransactionPosition(int64(index))
		entries = append(entries, entry)
	}
	return entries
}

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

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: MOVEMENT", func() {
	Describe("#ToMovementModel", func() {
		Context("success cases", func() {
			It("should map the movement identity and transaction ownership", func() {
				// Arrange.
				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries(historicalEntries()).Build()
				Expect(err).NotTo(HaveOccurred())

				// Act.
				persisted, err := model.ToMovementModel(movement, "019ff448-c43d-70d3-83c7-dfa067446b01")

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.ID).To(Equal(pgtype.Text{String: movement.ID.String(), Valid: true}))
				Expect(persisted.TransactionID).To(Equal(pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b01", Valid: true}))
				Expect(persisted.Type.String).To(Equal("TRANSFER"))
				Expect(persisted.Description.String).To(Equal("main transfer"))
			})
		})

		Context("error cases", func() {
			It("should reject a nil movement", func() {
				// Act.
				persisted, err := model.ToMovementModel(nil, "019ff448-c43d-70d3-83c7-dfa067446b01")

				// Assert.
				Expect(persisted).To(Equal(model.Movement{}))
				Expect(err).To(MatchError("movement cannot be nil"))
			})
		})
	})

	Describe("#ToMovementDomain", func() {
		Context("success cases", func() {
			It("should restore the movement and retain its entry references", func() {
				// Arrange.
				entries := historicalEntries()
				persisted := model.Movement{
					ID:          pgtype.Text{String: "019ff448-c43d-70d3-83c7-dfa067446b03", Valid: true},
					Type:        pgtype.Text{String: "TRANSFER", Valid: true},
					Description: pgtype.Text{String: "main transfer", Valid: true},
				}

				// Act.
				movement, err := model.ToMovementDomain(persisted, entries)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(movement.ID.String()).To(Equal(persisted.ID.String))
				Expect(movement.Type).To(Equal(persisted.Type.String))
				Expect(movement.Description).To(Equal(persisted.Description.String))
				Expect(movement.Entries).To(HaveLen(2))
				Expect(movement.Entries[0]).To(BeIdenticalTo(entries[0]))
			})
		})

		Context("error cases", func() {
			It("should reject an invalid persisted movement identity", func() {
				// Arrange.
				persisted := model.Movement{
					ID:          pgtype.Text{String: "invalid-id", Valid: true},
					Type:        pgtype.Text{String: "TRANSFER", Valid: true},
					Description: pgtype.Text{String: "main transfer", Valid: true},
				}

				// Act.
				movement, err := model.ToMovementDomain(persisted, historicalEntries())

				// Assert.
				Expect(movement).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
