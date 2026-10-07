//go:build unit
// +build unit

package transaction_test

import (
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: TRANSACTION", func() {
	Describe("#Build", func() {
		Context("success cases", func() {
			It("should allow debit and credit entries for the same account", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				const accountExternalID = "e4e5e6e7-e8e9-410e-a11e-e12e13e14e15"
				credit, err := transaction.NewEntryBuilder().
					WithID().
					WithAccountExternalID(accountExternalID).
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).NotTo(HaveOccurred())
				debit, err := transaction.NewEntryBuilder().
					WithID().
					WithAccountExternalID(accountExternalID).
					WithDirection(transaction.Debit).
					WithAmount(amount).
					Build()
				Expect(err).NotTo(HaveOccurred())

				built, err := transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("same-account-transfer").
					WithAmount(amount).
					WithOperation(transaction.OperationTransfer).
					WithEntries([]*transaction.Entry{credit, debit}).
					Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(built.Entries).To(HaveLen(2))
			})
		})
	})

	Describe("WithCreatedAt validation", func() {
		It("reports created_at for a future entry creation date", func() {
			amount, err := money.NewMoney(100, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			transactionID, err := entity.NewIDV7()
			Expect(err).NotTo(HaveOccurred())
			future := time.Now().AddDate(100, 0, 0)

			_, err = transaction.NewEntryBuilder().
				WithID().
				WithTransactionID(transactionID.String()).
				WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
				WithDirection(transaction.Credit).
				WithAmount(amount).
				WithCreatedAt(future).
				Build()

			var domainErr *fault.DomainError
			Expect(errors.As(err, &domainErr)).To(BeTrue())
			Expect(domainErr.Fields).To(HaveKeyWithValue("created_at", "cannot be in the future"))
			Expect(domainErr.Fields).NotTo(HaveKey("updated_at"))
		})

		It("reports created_at for a future transaction creation date", func() {
			amount, err := money.NewMoney(100, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			future := time.Now().AddDate(100, 0, 0)

			_, err = transaction.NewTransactionBuilder().
				WithID().
				WithIdempotencyKey("future-created-at-test").
				WithAmount(amount).
				WithOperation(transaction.OperationDeposit).
				WithCreatedAt(future).
				Build()

			var domainErr *fault.DomainError
			Expect(errors.As(err, &domainErr)).To(BeTrue())
			Expect(domainErr.Fields).To(HaveKeyWithValue("created_at", "cannot be in the future"))
			Expect(domainErr.Fields).NotTo(HaveKey("updated_at"))
		})
	})

	Describe("#NewTransactionBuilder", func() {
		Context("success cases", func() {
			It("should not return an error when build new transaction", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				trans, err := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					Build()
				Expect(err).To(BeNil())
				Expect(trans).NotTo(BeNil())
			})
		})
	})

	Describe("#WithEntries", func() {
		Context("success cases", func() {
			It("should balance each currency independently and preserve repeated account order", func() {
				brl, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				usd, err := money.NewMoney(200, money.USD)
				Expect(err).NotTo(HaveOccurred())
				entries := []*transaction.Entry{
					buildEntry(transaction.Debit, transferDebitAccount, brl),
					buildEntry(transaction.Credit, transferCreditAccount, brl),
					buildEntry(transaction.Credit, transferDebitAccount, usd),
					buildEntry(transaction.Debit, transferDebitAccount, usd),
				}

				built, err := transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("multi-currency-entries").
					WithAmount(brl).
					WithOperation(transaction.OperationTransfer).
					WithEntries(entries).
					Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(built.Entries).To(Equal(entries))
			})
		})

		Context("error cases", func() {
			It("should reject a transaction with a single entry", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				entry, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())

				trans, err := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithEntries([]*transaction.Entry{entry}).
					Build()

				Expect(err).To(HaveOccurred())
				Expect(trans).To(BeNil())
			})
		})

		Context("success cases", func() {
			It("should accept more than two balanced entries", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				amount200, _ := money.NewMoney(200, money.BRL)
				entry, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				entry2, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("f4f5f6f7-f8f9-410f-a11f-f12f13f14f15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				entry3, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("a4a5a6a7-a8a9-410a-a11a-a12a13a14a15").
					WithDirection(transaction.Debit).
					WithAmount(amount200).
					Build()
				Expect(err).To(BeNil())

				trans, err := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithEntries([]*transaction.Entry{entry, entry2, entry3}).
					Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(trans.Entries).To(HaveLen(3))
			})

			It("should reject entries without both directions", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				entry, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				entry2, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("f4f5f6f7-f8f9-410f-a11f-f12f13f14f15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())

				_, err = transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithEntries([]*transaction.Entry{entry, entry2}).
					Build()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("at least one debit and one credit"))
			})

			It("should return an error when add two entries with different amount", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				amount2, _ := money.NewMoney(200, money.BRL)
				entry, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				entry2, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("f4f5f6f7-f8f9-410f-a11f-f12f13f14f15").
					WithDirection(transaction.Debit).
					WithAmount(amount2).
					Build()
				Expect(err).To(BeNil())

				_, err = transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithEntries([]*transaction.Entry{entry, entry2}).
					Build()

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("balance by currency"))
			})
		})
	})

	Describe("#Complete", func() {
		Context("success cases", func() {
			It("should complete a transaction", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				trans, _ := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithStatus(transaction.Pending).
					Build()
				err := trans.Complete()
				Expect(err).To(BeNil())
				Expect(trans.Status).To(Equal(transaction.Completed))
			})
		})

		Context("error cases", func() {
			It("should return an error when transaction is already completed", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				trans, _ := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithStatus(transaction.Completed).
					Build()
				err := trans.Complete()
				Expect(err).NotTo(BeNil())
				Expect(err).To(Equal(transaction.ErrInvalidTransactionStatus))
			})
		})
	})

	Describe("#Fail", func() {
		Context("success cases", func() {
			It("should fail a transaction", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				trans, _ := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithStatus(transaction.Pending).
					Build()
				err := trans.Fail()
				Expect(err).To(BeNil())
				Expect(trans.Status).To(Equal(transaction.Failed))
			})
		})

		Context("error cases", func() {
			It("should return an error when transaction is already completed", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				trans, _ := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithStatus(transaction.Completed).
					Build()
				err := trans.Fail()
				Expect(err).NotTo(BeNil())
				Expect(err).To(Equal(transaction.ErrInvalidTransactionStatus))
			})
		})
	})

	Describe("#IsValid", func() {
		Context("success cases", func() {
			It("should return true for valid status", func() {
				Expect(transaction.Pending.IsValid()).To(BeTrue())
				Expect(transaction.Completed.IsValid()).To(BeTrue())
				Expect(transaction.Failed.IsValid()).To(BeTrue())
			})
		})

		Context("error cases", func() {
			It("should return false for invalid status", func() {
				Expect(transaction.TransactionStatus("invalid").IsValid()).To(BeFalse())
			})
		})
	})

	Describe("#NewEntryBuilder", func() {
		Context("success cases", func() {
			It("should create a new entry", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				entry, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				Expect(entry).NotTo(BeNil())
			})
		})

		Context("error cases", func() {
			It("should return an error when direction is invalid", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(100, money.BRL)
				_, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection("invalid").
					WithAmount(amount).
					Build()
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid direction"))
			})

			It("should return an error when amount is zero", func() {
				id, _ := entity.NewIDV7()
				amount, _ := money.NewMoney(0, money.BRL)
				_, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					Build()
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("cannot be zero"))
			})

			It("should return an error when amount is negative", func() {
				id, _ := entity.NewIDV7()
				amount1, _ := money.NewMoney(100, money.BRL)
				amount2, _ := money.NewMoney(200, money.BRL)
				negativeAmount, _ := amount1.Subtract(amount2)
				_, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("e4e5e6e7-e8e9-410e-a11e-e12e13e14e15").
					WithDirection(transaction.Credit).
					WithAmount(negativeAmount).
					Build()
				Expect(err.Error()).To(ContainSubstring("cannot be negative"))
			})
		})
	})

	Describe("#Direction.IsValid", func() {
		Context("success cases", func() {
			It("should return true for valid directions", func() {
				Expect(transaction.Credit.IsValid()).To(BeTrue())
				Expect(transaction.Debit.IsValid()).To(BeTrue())
			})
		})

		Context("error cases", func() {
			It("should return false for invalid direction", func() {
				Expect(transaction.Direction("invalid").IsValid()).To(BeFalse())
			})
		})
	})
})
