//go:build unit

package tax_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/tax"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TAX :: TAX", func() {
	Describe("#NewCNPJOrCPF", func() {
		Context("success cases", func() {
			It("should normalize and validate a CPF", func() {
				// Arrange (Given)
				raw := "529.982.247-25"

				// Act (When)
				value, eval := tax.NewCNPJOrCPF(raw)

				// Assert (Then)
				Expect(eval).To(BeEmpty())
				Expect(value.String()).To(Equal("52998224725"))
			})

			It("should normalize and validate a CNPJ", func() {
				// Arrange (Given)
				raw := "04.252.011/0001-10"

				// Act (When)
				value, eval := tax.NewCNPJOrCPF(raw)

				// Assert (Then)
				Expect(eval).To(BeEmpty())
				Expect(value.String()).To(Equal("04252011000110"))
			})
		})

		Context("error cases", func() {
			It("should reject blank, blacklisted, and invalid identifiers", func() {
				// Arrange (Given)
				values := []string{"", "111.111.111-11", "529.982.247-26", "11.111.111/1111-11"}

				// Act (When) and Assert (Then)
				for _, raw := range values {
					value, eval := tax.NewCNPJOrCPF(raw)
					Expect(value).To(BeNil(), raw)
					Expect(eval).NotTo(BeEmpty(), raw)
				}
			})
		})
	})
})
