//go:build unit

package dto_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = It("builds the account domain and masks sensitive fields", func() {
	input := dto.CreateAccountInput{
		AccountExternalID: uuid.NewString(),
		AccountNumber:     "1234567890",
		TaxID:             "52998224725",
		AccountType:       "ASSET",
		BalancePolicy:     "BALANCE_NON_NEGATIVE",
		Currency:          "BRL",
	}

	account, err := input.CreateAccountFacade()
	Expect(err).NotTo(HaveOccurred())
	Expect(account.AccountExternalID).To(Equal(input.AccountExternalID))
	Expect(account.AccountNumber).To(Equal(input.AccountNumber))
	Expect(string(account.Currency)).To(Equal(input.Currency))
	Expect(string(account.BalancePolicy)).To(Equal(input.BalancePolicy))

	Expect(dto.MaskTotal(input.AccountNumber)).To(Equal("**********"))
	Expect(dto.MaskMiddleVisible("1234")).To(Equal("******"))
	Expect(dto.MaskMiddleVisible("1234567")).To(Equal("**34***"))
})

var _ = It("rejects an absent or unknown balance policy", func() {
	input := dto.CreateAccountInput{
		AccountExternalID: uuid.NewString(),
		AccountNumber:     "1234567890",
		TaxID:             "52998224725",
		AccountType:       "ASSET",
		Currency:          "BRL",
	}

	_, err := input.CreateAccountFacade()
	Expect(err).To(HaveOccurred())

	input.BalancePolicy = "BALANCE_UNKNOWN"
	_, err = input.CreateAccountFacade()
	Expect(err).To(HaveOccurred())
})
