//go:build unit

package validator_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = It("validates primitive values", func() {
	Expect(validator.NotBlank("value")).To(BeTrue())
	Expect(validator.NotBlank("   ")).To(BeFalse())
	Expect(validator.MaxChars("ábc", 3)).To(BeTrue())
	Expect(validator.MaxChars("abcd", 3)).To(BeFalse())
	Expect(validator.MinChars("ábc", 3)).To(BeTrue())
	Expect(validator.MinChars("ab", 3)).To(BeFalse())
	Expect(validator.MatchesUUID("d589965c-1622-4329-98f9-f13354a2e4dc")).To(BeTrue())
	Expect(validator.MatchesUUIDv7("019ff448-c43d-70d3-83c7-dfa0674469b7")).To(BeTrue())
	Expect(validator.MatchesUUIDv7("d589965c-1622-4329-98f9-f13354a2e4dc")).To(BeFalse())
})
