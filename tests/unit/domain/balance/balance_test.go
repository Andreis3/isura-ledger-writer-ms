//go:build unit

package balance_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("balance builder", func() {
	It("builds with explicit values", func() {
		id := uuid.NewString()
		accountID := uuid.NewString()
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

		result, err := balance.NewBalanceBuilder().
			WithID(id).
			WithAccountID(accountID).
			WithAmount(1500, money.BRL).
			WithCreatedAt(now).
			WithUpdatedAt(now).
			Build()

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ID().String()).To(Equal(id))
		Expect(result.AccountID()).To(Equal(accountID))
		Expect(result.Amount().Amount()).To(Equal(int64(1500)))
		Expect(result.CreatedAT()).To(Equal(now))
		Expect(result.UpdatedAT()).To(Equal(now))
	})

	It("uses defaults", func() {
		result, err := balance.NewBalanceBuilder().
			WithID().
			WithAccountID(uuid.NewString()).
			WithAmount(0, money.BRL).
			WithCreatedAt().
			WithUpdatedAt().
			Build()

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ID().String()).NotTo(BeEmpty())
		Expect(result.CreatedAT()).NotTo(BeZero())
		Expect(result.UpdatedAT()).NotTo(BeZero())
	})
})
