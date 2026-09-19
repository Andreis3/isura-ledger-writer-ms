//go:build unit
// +build unit

package transaction_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: TRANSACTION", func() {
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
			It("should not return an error when add new entry", func() {
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

				Expect(err).To(BeNil())
				Expect(trans.Entries).To(HaveLen(1))
			})
		})

		Context("error cases", func() {
			It("should return an error when add more than two entries", func() {
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
					WithDirection(transaction.Debit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())
				entry3, err := transaction.NewEntryBuilder().
					WithID().
					WithTransactionID(id.String()).
					WithAccountExternalID("a4a5a6a7-a8a9-410a-a11a-a12a13a14a15").
					WithDirection(transaction.Debit).
					WithAmount(amount).
					Build()
				Expect(err).To(BeNil())

				_, err = transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("any_idempotency_key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithEntries([]*transaction.Entry{entry, entry2, entry3}).
					Build()

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("maximum entries exceeded"))
			})

			It("should return an error when add two entries with same direction", func() {
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

				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("duplicate entry direction"))
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
				Expect(err.Error()).To(ContainSubstring("different amount"))
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
