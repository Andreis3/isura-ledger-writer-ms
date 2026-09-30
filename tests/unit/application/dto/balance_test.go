//go:build unit

package dto_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: DTO :: BALANCE", func() {
	Describe("#NewBalanceDomain", func() {
		Context("success cases", func() {
			It("should build a zero balance for the requested account", func() {
				// Arrange (Given)
				accountID := "01936f7e-6f1d-7b13-9b5f-4c2e0b8c6d11"
				input := &dto.CreateBalanceInput{AccountID: accountID, Currency: "BRL"}

				// Act (When)
				result, err := input.NewBalanceDomain()

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.AccountID()).To(Equal(accountID))
				Expect(result.Amount().Amount()).To(BeZero())
				Expect(string(result.Amount().Currency())).To(Equal("BRL"))
			})
		})
	})
})
