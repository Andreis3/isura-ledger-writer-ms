//go:build unit

package shared_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: SHARED :: UTILS", func() {
	Describe("#CoalesceTime", func() {
		Context("success cases", func() {
			It("should return the value when present and the fallback when absent", func() {
				// Arrange (Given)
				fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				value := fallback.Add(time.Hour)

				// Act (When)
				present := shared.CoalesceTime(value, fallback)
				absent := shared.CoalesceTime(time.Time{}, fallback)

				// Assert (Then)
				Expect(present).To(Equal(value))
				Expect(absent).To(Equal(fallback))
			})
		})
	})
})
