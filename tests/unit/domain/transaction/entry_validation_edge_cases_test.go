//go:build unit

package transaction_test

import (
 "math"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: TRANSACTION", func() {
 Describe("#WithEntries", func() {
  Context("error cases", func() {
   It("should reject nil postings rather than accept an incomplete composition", func() {
    // Arrange.
    amount, err := money.NewMoney(100, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    valid := buildEntry(transaction.Debit, transferDebitAccount, amount)

    // Act.
    built, err := transaction.NewTransactionBuilder().WithID().
     WithIdempotencyKey("nil-posting").
     WithAmount(amount).
     WithOperation(transaction.OperationTransfer).
     WithEntries([]*transaction.Entry{valid, nil}).
     Build()

    // Assert.
    Expect(built).To(BeNil())
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("entry cannot be nil"))
   })

   It("should reject a debit total that overflows int64", func() {
    // Arrange.
    largest, err := money.NewMoney(math.MaxInt64, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    one, err := money.NewMoney(1, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    entries := []*transaction.Entry{
     buildEntry(transaction.Debit, transferDebitAccount, largest),
     buildEntry(transaction.Debit, transferDebitAccount, one),
     buildEntry(transaction.Credit, transferCreditAccount, largest),
    }

    // Act.
    built, err := transaction.NewTransactionBuilder().WithID().
     WithIdempotencyKey("debit-overflow").
     WithAmount(largest).WithOperation(transaction.OperationTransfer).
     WithEntries(entries).Build()

    // Assert.
    Expect(built).To(BeNil())
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("debit total overflows"))
   })

   It("should reject a credit total that overflows int64", func() {
    // Arrange.
    largest, err := money.NewMoney(math.MaxInt64, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    one, err := money.NewMoney(1, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    entries := []*transaction.Entry{
     buildEntry(transaction.Credit, transferCreditAccount, largest),
     buildEntry(transaction.Credit, transferCreditAccount, one),
     buildEntry(transaction.Debit, transferDebitAccount, largest),
    }

    // Act.
    built, err := transaction.NewTransactionBuilder().WithID().
     WithIdempotencyKey("credit-overflow").
     WithAmount(largest).WithOperation(transaction.OperationTransfer).
     WithEntries(entries).Build()

    // Assert.
    Expect(built).To(BeNil())
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("credit total overflows"))
   })
  })
 })
})
