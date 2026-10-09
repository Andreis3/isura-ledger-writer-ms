//go:build unit

package account_test

import (
 "errors"
 "math"

 "github.com/andreis3/isura-ledger-ms/internal/domain/account"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: ACCOUNT :: LEDGER", func() {
 Describe("#ApplyEntry", func() {
  Context("error cases", func() {
   It("should reject a running balance overflow without changing the original state", func() {
    // Arrange.
    value, err := money.NewMoney(2, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL}
    initial := account.LedgerState{SequenceNumber: 12, RunningBalance: math.MaxInt64 - 1}

    // Act.
    next, err := accountUnderTest.ApplyEntry(initial, transaction.Debit, value)

    // Assert.
    Expect(errors.Is(err, account.ErrLedgerBalanceOverflow)).To(BeTrue())
    Expect(next).To(Equal(initial))
   })

   It("should reject a running balance underflow without changing the original state", func() {
    // Arrange.
    value, err := money.NewMoney(2, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL}
    initial := account.LedgerState{SequenceNumber: 12, RunningBalance: math.MinInt64 + 1}

    // Act.
    next, err := accountUnderTest.ApplyEntry(initial, transaction.Credit, value)

    // Assert.
    Expect(errors.Is(err, account.ErrLedgerBalanceOverflow)).To(BeTrue())
    Expect(next).To(Equal(initial))
   })

   It("should reject a sequence overflow without changing the original state", func() {
    // Arrange.
    value, err := money.NewMoney(1, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL}
    initial := account.LedgerState{SequenceNumber: math.MaxInt64, RunningBalance: 10}

    // Act.
    next, err := accountUnderTest.ApplyEntry(initial, transaction.Debit, value)

    // Assert.
    Expect(errors.Is(err, account.ErrLedgerBalanceOverflow)).To(BeTrue())
    Expect(next).To(Equal(initial))
   })

   It("should reject an unsupported accounting type without updating state", func() {
    // Arrange.
    value, err := money.NewMoney(1, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Type("UNKNOWN"), BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL}
    initial := account.LedgerState{SequenceNumber: 2, RunningBalance: 10}

    // Act.
    next, err := accountUnderTest.ApplyEntry(initial, transaction.Debit, value)

    // Assert.
    Expect(errors.Is(err, account.ErrInvalidAccountingType)).To(BeTrue())
    Expect(next).To(Equal(initial))
   })

   It("should reject a negative initial sequence without changing the original state", func() {
    // Arrange.
    value, err := money.NewMoney(1, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL}
    initial := account.LedgerState{SequenceNumber: -1, RunningBalance: 10}

    // Act.
    next, err := accountUnderTest.ApplyEntry(initial, transaction.Debit, value)

    // Assert.
    Expect(errors.Is(err, account.ErrInvalidLedgerState)).To(BeTrue())
    Expect(next).To(Equal(initial))
   })
  })
 })
 Describe("#ApplyHistoricalEntry", func() {
  Context("error cases", func() {
   It("should reject invalid balance policy even for historical reconstruction", func() {
    // Arrange.
    value, err := money.NewMoney(5, money.BRL)
    Expect(err).NotTo(HaveOccurred())
    accountUnderTest := account.Account{AccountType: account.Asset, BalancePolicy: account.BalancePolicy("UNKNOWN"), Currency: money.BRL}
    initial := account.LedgerState{RunningBalance: -10}

    // Act.
    next, err := accountUnderTest.ApplyHistoricalEntry(initial, transaction.Debit, value)

    // Assert.
    Expect(err).To(HaveOccurred())
    Expect(next).To(Equal(initial))
   })
  })
 })
})
