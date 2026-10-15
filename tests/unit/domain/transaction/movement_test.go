//go:build unit
// +build unit

package transaction_test

import (
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: MOVEMENT", func() {
	Describe("#NewMovementBuilder", func() {
		Context("success cases", func() {
			It("should build a movement with a generated stable identifier", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entry := buildEntry(transaction.Debit, transferDebitAccount, amount)

				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{entry}).Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(movement.ID.String()).NotTo(BeEmpty())
				Expect(movement.Entries).To(ConsistOf(entry))
			})

			It("should accept a supplied UUIDv7 identifier", func() {
				id, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entry := buildEntry(transaction.Debit, transferDebitAccount, amount)

				movement, err := transaction.NewMovementBuilder().WithID(id.String()).WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{entry}).Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(movement.ID).To(Equal(id))
			})
		})

		Context("error cases", func() {
			It("should reject missing movement fields and entries", func() {
				_, err := transaction.NewMovementBuilder().WithID().WithType(" ").
					WithDescription("\t").WithEntries(nil).Build()

				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("#TransactionBuilder.WithMovements", func() {
		Context("success cases", func() {
			It("should accept movements that partition and balance transaction entries", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				debit := buildEntry(transaction.Debit, transferDebitAccount, amount)
				credit := buildEntry(transaction.Credit, transferCreditAccount, amount)
				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{debit, credit}).Build()
				Expect(err).NotTo(HaveOccurred())

				built, err := transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(amount).WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{debit, credit}).WithMovements([]*transaction.Movement{movement}).Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(built.Movements).To(ConsistOf(movement))
			})

			It("should allow an empty movement collection for historical transactions", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entries := []*transaction.Entry{
					buildEntry(transaction.Debit, transferDebitAccount, amount),
					buildEntry(transaction.Credit, transferCreditAccount, amount),
				}

				built, err := transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(amount).WithOperation(transaction.OperationTransfer).WithEntries(entries).Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(built.Movements).To(BeNil())
			})
		})

		Context("error cases", func() {
			It("should reject entries omitted from the movement partition", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				debit := buildEntry(transaction.Debit, transferDebitAccount, amount)
				credit := buildEntry(transaction.Credit, transferCreditAccount, amount)
				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{debit}).Build()
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(amount).WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{debit, credit}).WithMovements([]*transaction.Movement{movement}).Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject an unbalanced movement", func() {
				debitAmount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				creditAmount, err := money.NewMoney(50, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				debit := buildEntry(transaction.Debit, transferDebitAccount, debitAmount)
				credit := buildEntry(transaction.Credit, transferCreditAccount, creditAmount)
				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{debit, credit}).Build()
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(debitAmount).WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{debit, credit}).WithMovements([]*transaction.Movement{movement}).Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject duplicate and foreign entry references", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				debit := buildEntry(transaction.Debit, transferDebitAccount, amount)
				credit := buildEntry(transaction.Credit, transferCreditAccount, amount)
				foreign := buildEntry(transaction.Credit, transferCreditAccount, amount)
				movement, err := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
					WithDescription("main transfer").WithEntries([]*transaction.Entry{debit, credit, foreign}).Build()
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(amount).WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{debit, credit}).WithMovements([]*transaction.Movement{movement}).Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject an entry assigned to more than one movement", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				debit := buildEntry(transaction.Debit, transferDebitAccount, amount)
				credit := buildEntry(transaction.Credit, transferCreditAccount, amount)
				buildMovement := func() *transaction.Movement {
					movement, buildErr := transaction.NewMovementBuilder().WithID().WithType("TRANSFER").
						WithDescription("main transfer").WithEntries([]*transaction.Entry{debit, credit}).Build()
					Expect(buildErr).NotTo(HaveOccurred())
					return movement
				}

				_, err = transaction.NewTransactionBuilder().WithID().WithIdempotencyKey("key").
					WithAmount(amount).WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{debit, credit}).
					WithMovements([]*transaction.Movement{buildMovement(), buildMovement()}).Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject blank movement type and description", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entry := buildEntry(transaction.Debit, transferDebitAccount, amount)

				_, err = transaction.NewMovementBuilder().WithID().WithType(strings.Repeat(" ", 2)).
					WithDescription(" ").WithEntries([]*transaction.Entry{entry}).Build()

				Expect(err).To(HaveOccurred())
			})
		})
	})
})
