//go:build unit

package entity_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: ENTITY :: ID", func() {
	Describe("#NewID", func() {
		Context("success cases", func() {
			It("should preserve a valid identifier", func() {
				// Arrange (Given)
				value := "d589965c-1622-4329-98f9-f13354a2e4dc"

				// Act (When)
				id, err := entity.NewID(value)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(id.String()).To(Equal(value))
			})
		})
		Context("error cases", func() {
			It("should reject an invalid identifier", func() {
				// Arrange (Given)
				value := "invalid"

				// Act (When)
				_, err := entity.NewID(value)

				// Assert (Then)
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Describe("#NewIDV7", func() {
		Context("success cases", func() {
			It("should generate a non-empty identifier", func() {
				// Arrange (Given)

				// Act (When)
				id, err := entity.NewIDV7()

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(id.String()).NotTo(BeEmpty())
			})
		})
	})
})
