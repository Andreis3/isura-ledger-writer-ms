//go:build unit
// +build unit

package account_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

var _ = Describe("INTERNAL :: DOMAIN :: ACCOUNT :: ACCOUNT", func() {
	Describe("#NewAccountBuilder", func() {
		Context("success cases", func() {
			It("should not return an error when build new account", func() {
				acc, err := account.NewAccountBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithAccountNumber("123456").
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

})
