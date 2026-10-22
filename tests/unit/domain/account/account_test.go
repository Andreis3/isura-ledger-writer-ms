//go:build unit
// +build unit

package account_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

var _ = Describe("INTERNAL :: DOMAIN :: ACCOUNT :: ACCOUNT", func() {
	Describe("#Status.IsValid", func() {
		Context("success cases", func() {
			It("should accept each supported account status", func() {
				Expect(account.StatusActive.IsValid()).To(BeTrue())
				Expect(account.StatusBlocked.IsValid()).To(BeTrue())
				Expect(account.StatusClosed.IsValid()).To(BeTrue())
			})
		})
		Context("error cases", func() {
			It("should reject an unknown account status", func() {
				Expect(account.Status("UNKNOWN").IsValid()).To(BeFalse())
			})
		})
	})

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

	Describe("#WithCreatedAt and #WithUpdatedAt", func() {
		Context("success cases", func() {
			It("should assign current timestamps when no explicit value is supplied", func() {
				builder := validAccountBuilder().WithCreatedAt().WithUpdatedAt()
				acc, err := builder.Build()
				Expect(err).NotTo(HaveOccurred())
				Expect(acc.CreatedAt).NotTo(BeZero())
				Expect(acc.UpdatedAt).NotTo(BeZero())
			})
		})
		Context("error cases", func() {
			It("should reject future account timestamps with the corresponding field errors", func() {
				future := time.Now().Add(time.Hour)
				_, createdErr := validAccountBuilder().WithCreatedAt(future).Build()
				var createdDomainErr *fault.DomainError
				Expect(errors.As(createdErr, &createdDomainErr)).To(BeTrue())
				Expect(createdDomainErr.Fields).To(HaveKeyWithValue("created_at", "cannot be in the future"))

				_, updatedErr := validAccountBuilder().WithUpdatedAt(future).Build()
				var updatedDomainErr *fault.DomainError
				Expect(errors.As(updatedErr, &updatedDomainErr)).To(BeTrue())
				Expect(updatedDomainErr.Fields).To(HaveKeyWithValue("updated_at", "cannot be in the future"))
			})
		})
	})

	Describe("#WithHoldsEligible", func() {
		Context("success cases", func() {
			It("should keep hold eligibility disabled unless explicitly enabled", func() {
				eligible, err := account.NewAccountBuilder().WithID().WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").WithAccountNumber("123456").WithBalancePolicy("BALANCE_NON_NEGATIVE").WithCurrency("BRL").WithHoldsEligible(true).Build()
				Expect(err).NotTo(HaveOccurred())
				Expect(eligible.HoldsEligible).To(BeTrue())
				ineligible, err := account.NewAccountBuilder().WithID().WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").WithAccountNumber("123456").WithBalancePolicy("BALANCE_NON_NEGATIVE").WithCurrency("BRL").Build()
				Expect(err).NotTo(HaveOccurred())
				Expect(ineligible.HoldsEligible).To(BeFalse())
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

		It("recalculates historical restrictive entries without retroactive rejection", func() {
			amount, err := money.NewMoney(101, money.BRL)
			Expect(err).NotTo(HaveOccurred())
			acc := account.Account{AccountType: account.Asset, Currency: money.BRL, BalancePolicy: account.BalanceNonNegative}

			state, applyErr := acc.ApplyHistoricalEntry(account.LedgerState{RunningBalance: 100}, transaction.Credit, amount)

			Expect(applyErr).NotTo(HaveOccurred())
			Expect(state.SequenceNumber).To(Equal(int64(1)))
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

func validAccountBuilder() *account.AccountBuilder {
	return account.NewAccountBuilder().WithID().WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
		WithAccountNumber("123456").WithBalancePolicy(string(account.BalanceNonNegative)).WithCurrency(string(money.BRL))
}
