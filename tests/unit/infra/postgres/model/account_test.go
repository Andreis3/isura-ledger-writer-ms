//go:build unit

package model_test

import (
	"testing"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAccountModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Account Model Suite")
}

var _ = Describe("Postgres account model", func() {
	It("preserves the balance policy in both directions", func() {
		entity, err := account.NewAccountBuilder().
			WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
			WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
			WithAccountNumber("123456").
			WithTaxID("529.982.247-25").
			WithStatus().
			WithType(string(account.Asset)).
			WithBalancePolicy(string(account.BalanceUnrestricted)).
			WithCurrency(string(money.BRL)).
			Build()
		Expect(err).NotTo(HaveOccurred())

		mapped := model.ToAccountModel(entity)
		Expect(mapped.BalancePolicy.String).To(Equal(string(account.BalanceUnrestricted)))

		decoded, err := model.ToAccountDomain(mapped)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.BalancePolicy).To(Equal(account.BalanceUnrestricted))
	})
})
