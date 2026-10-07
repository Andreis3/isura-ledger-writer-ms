//go:build unit
// +build unit

package transaction_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: ENTRY", func() {
	Describe("#Direction.String", func() {
		Context("success cases", func() {
			It("should return CREDIT for the credit direction", func() {
				// Arrange
				direction := transaction.Credit

				// Act
				result := direction.String()

				// Assert
				Expect(result).To(Equal("CREDIT"))
			})

			It("should return DEBIT for the debit direction", func() {
				// Arrange
				direction := transaction.Debit

				// Act
				result := direction.String()

				// Assert
				Expect(result).To(Equal("DEBIT"))
			})

			It("should preserve an unknown direction value", func() {
				// Arrange
				const unknownDirection = "UNKNOWN"
				direction := transaction.Direction(unknownDirection)

				// Act
				result := direction.String()

				// Assert
				Expect(result).To(Equal(unknownDirection))
			})
		})
	})
})
