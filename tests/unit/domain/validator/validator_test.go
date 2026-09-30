//go:build unit

package validator_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: VALIDATOR :: VALIDATOR", func() {
	Describe("#NotBlank", func() {
		Context("success cases", func() {
			It("should accept non-blank values and reject whitespace", func() {
				// Arrange (Given)
				value := "value"

				// Act (When)
				valid := validator.NotBlank(value)
				blank := validator.NotBlank("   ")

				// Assert (Then)
				Expect(valid).To(BeTrue())
				Expect(blank).To(BeFalse())
			})
		})
	})
	Describe("#MaxChars", func() {
		Context("success cases", func() {
			It("should count unicode characters against the maximum", func() {
				// Arrange (Given)

				// Act (When)
				withinLimit := validator.MaxChars("ábc", 3)
				overLimit := validator.MaxChars("abcd", 3)

				// Assert (Then)
				Expect(withinLimit).To(BeTrue())
				Expect(overLimit).To(BeFalse())
			})
		})
	})
	Describe("#MinChars", func() {
		Context("success cases", func() {
			It("should enforce the minimum character count", func() {
				// Arrange (Given)

				// Act (When)
				withinLimit := validator.MinChars("ábc", 3)
				underLimit := validator.MinChars("ab", 3)

				// Assert (Then)
				Expect(withinLimit).To(BeTrue())
				Expect(underLimit).To(BeFalse())
			})
		})
	})
	Describe("#MatchesUUID", func() {
		Context("success cases", func() {
			It("should accept a valid UUID", func() {
				// Arrange (Given)
				value := "d589965c-1622-4329-98f9-f13354a2e4dc"

				// Act (When)
				valid := validator.MatchesUUID(value)

				// Assert (Then)
				Expect(valid).To(BeTrue())
			})
		})
	})
	Describe("#MatchesUUIDv7", func() {
		Context("success cases", func() {
			It("should accept version 7 identifiers and reject other versions", func() {
				// Arrange (Given)
				validID := "019ff448-c43d-70d3-83c7-dfa0674469b7"
				invalidID := "d589965c-1622-4329-98f9-f13354a2e4dc"

				// Act (When)
				valid := validator.MatchesUUIDv7(validID)
				invalid := validator.MatchesUUIDv7(invalidID)

				// Assert (Then)
				Expect(valid).To(BeTrue())
				Expect(invalid).To(BeFalse())
			})
		})
	})
})
