//go:build unit

package dto_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = It("builds a zero balance domain", func() {
	accountID := "01936f7e-6f1d-7b13-9b5f-4c2e0b8c6d11"
	result, err := (&dto.CreateBalanceInput{AccountID: accountID, Currency: "BRL"}).NewBalanceDomain()

	Expect(err).NotTo(HaveOccurred())
	Expect(result.AccountID()).To(Equal(accountID))
	Expect(result.Amount().Amount()).To(BeZero())
	Expect(string(result.Amount().Currency())).To(Equal("BRL"))
})
