//go:build unit
// +build unit

package account_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

var _ = Describe("INTERNAL :: DOMAIN :: ACCOUNT :: ACCOUNT", func() {
	Describe("#NewAccountBuilder", func() {
		Context("success cases", func() {
			It("should not return an error when build new account", func() {
				acc, err := account.NewAccountBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountNumber("123456").
					WithBalancePolicy(string(account.BalanceNonNegative)).
					WithCurrency(string(money.BRL)).
					Build()
				Expect(err).To(BeNil())
				Expect(acc).NotTo(BeNil())
			})

			It("accepts valid status and accounting type", func() {
				acc, err := account.NewAccountBuilder().
					WithID().
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountNumber("123456").
					WithTaxID("529.982.247-25").
					WithStatus(string(account.StatusBlocked)).
					WithType(string(account.Liability)).
					WithBalancePolicy(string(account.BalanceNonNegative)).
					WithCurrency(string(money.BRL)).
					Build()
				Expect(err).NotTo(HaveOccurred())
				Expect(acc.Status).To(Equal(account.StatusBlocked))
				Expect(acc.AccountType).To(Equal(account.Liability))
			})
		})

		Context("error cases", func() {

			It("should return an error when external id is empty", func() {
				_, err := account.NewAccountBuilder().
					WithID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountExternalID("").
					WithAccountNumber("123456").
					WithBalancePolicy(string(account.BalanceNonNegative)).
					WithCurrency(string(money.BRL)).
					Build()
				Expect(err).NotTo(BeNil())
			})

			It("should return an error when currency is invalid", func() {
				_, err := account.NewAccountBuilder().
					WithID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountNumber("123456").
					WithCurrency("INVALID").
					Build()
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("#ApplyEntry", func() {
		It("applies debit and credit using the normal balance of each accounting type", func() {
			amount, err := money.NewMoney(100, money.BRL)
			Expect(err).NotTo(HaveOccurred())

			debitIncreases := []account.Type{account.Asset, account.Expense}
			creditIncreases := []account.Type{account.Liability, account.Revenue, account.Equity}
			for _, accountingType := range debitIncreases {
				acc := account.Account{AccountType: accountingType, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}
				state, applyErr := acc.ApplyEntry(account.LedgerState{RunningBalance: 100}, transaction.Debit, amount)
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(state.RunningBalance).To(Equal(int64(200)))
				state, applyErr = acc.ApplyEntry(account.LedgerState{RunningBalance: 100}, transaction.Credit, amount)
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(state.RunningBalance).To(Equal(int64(0)))
			}
			for _, accountingType := range creditIncreases {
				acc := account.Account{AccountType: accountingType, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}
				state, applyErr := acc.ApplyEntry(account.LedgerState{RunningBalance: 100}, transaction.Credit, amount)
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(state.RunningBalance).To(Equal(int64(200)))
				state, applyErr = acc.ApplyEntry(account.LedgerState{RunningBalance: 100}, transaction.Debit, amount)
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(state.RunningBalance).To(Equal(int64(0)))
			}
		})

		It("rejects a restrictive entry that would make the normal balance negative", func() {
			amount, err := money.NewMoney(101, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			acc := account.Account{AccountType: account.Asset, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}
			initial := account.LedgerState{SequenceNumber: 7, RunningBalance: 100}

			state, applyErr := acc.ApplyEntry(initial, transaction.Credit, amount)

			Expect(applyErr).To(HaveOccurred())
			Expect(errors.Is(applyErr, fault.ErrInsufficientBalance)).To(BeTrue())
			Expect(state).To(Equal(initial))
		})

		It("allows a negative normal balance when unrestricted", func() {
			amount, err := money.NewMoney(101, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			acc := account.Account{AccountType: account.Asset, Currency: money.BRL, BalancePolicy: account.BalanceUnrestricted}

			state, applyErr := acc.ApplyEntry(account.LedgerState{RunningBalance: 100}, transaction.Credit, amount)

			Expect(applyErr).NotTo(HaveOccurred())
			Expect(state.RunningBalance).To(Equal(int64(-1)))
		})

		It("rejects invalid policy, direction, amount and currency without mutating state", func() {
			validAmount, err := money.NewMoney(100, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			initial := account.LedgerState{SequenceNumber: 3, RunningBalance: 50}

			_, applyErr := (account.Account{AccountType: account.Asset, Currency: money.BRL}).ApplyEntry(initial, transaction.Debit, validAmount)
			Expect(errors.Is(applyErr, fault.ErrInvalidBalancePolicy)).To(BeTrue())

			acc := account.Account{AccountType: account.Asset, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}
			_, applyErr = acc.ApplyEntry(initial, transaction.Direction("UNKNOWN"), validAmount)
			Expect(errors.Is(applyErr, transaction.ErrInvalidDirection)).To(BeTrue())

			_, applyErr = acc.ApplyEntry(initial, transaction.Debit, money.Money{})
			Expect(errors.Is(applyErr, transaction.ErrAmountEqualZero)).To(BeTrue())

			usdAmount, amountErr := money.NewMoney(100, money.USD)
			Expect(amountErr).NotTo(HaveOccurred())
			_, applyErr = acc.ApplyEntry(initial, transaction.Debit, usdAmount)
			Expect(errors.Is(applyErr, money.ErrCurrencyMismatch)).To(BeTrue())
		})

		It("increments the sequence exactly once and preserves the input state", func() {
			amount, err := money.NewMoney(25, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			acc := account.Account{AccountType: account.Asset, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}
			initial := account.LedgerState{AccountID: "account-1", SequenceNumber: 9, RunningBalance: 100}

			state, applyErr := acc.ApplyEntry(initial, transaction.Debit, amount)

			Expect(applyErr).NotTo(HaveOccurred())
			Expect(state.SequenceNumber).To(Equal(int64(10)))
			Expect(initial).To(Equal(account.LedgerState{AccountID: "account-1", SequenceNumber: 9, RunningBalance: 100}))
		})
	})

})
