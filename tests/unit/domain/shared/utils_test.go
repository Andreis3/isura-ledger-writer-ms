//go:build unit

package shared_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = It("uses the value when present and fallback otherwise", func() {
	fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	value := fallback.Add(time.Hour)

	Expect(shared.CoalesceTime(value, fallback)).To(Equal(value))
	Expect(shared.CoalesceTime(time.Time{}, fallback)).To(Equal(fallback))
})
