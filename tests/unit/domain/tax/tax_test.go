//go:build unit

package tax_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/tax"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tax identifier", func() {
	It("normalizes and validates CPF", func() {
		value, eval := tax.NewCNPJOrCPF("529.982.247-25")

		Expect(eval).To(BeEmpty())
		Expect(value.String()).To(Equal("52998224725"))
	})

	It("normalizes and validates CNPJ", func() {
		value, eval := tax.NewCNPJOrCPF("04.252.011/0001-10")

		Expect(eval).To(BeEmpty())
		Expect(value.String()).To(Equal("04252011000110"))
	})

	It("rejects blank, blacklisted and invalid values", func() {
		for _, raw := range []string{"", "111.111.111-11", "529.982.247-26", "11.111.111/1111-11"} {
			value, eval := tax.NewCNPJOrCPF(raw)
			Expect(value).To(BeNil(), raw)
			Expect(eval).NotTo(BeEmpty(), raw)
		}
	})
})
