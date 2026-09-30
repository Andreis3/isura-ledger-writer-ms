//go:build unit

package balance_test

import (
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("balance builder", func() {
	It("associates a future creation date error with created_at", func() {
		future := time.Now().AddDate(100, 0, 0)
		entity, err := balance.NewBalanceBuilder().WithCreatedAt(future).Build()

		var domainErr *fault.DomainError
		Expect(errors.As(err, &domainErr)).To(BeTrue())
		Expect(entity).To(BeNil())
		Expect(domainErr.Fields).To(HaveKeyWithValue("created_at", "cannot be in the future"))
		Expect(domainErr.Fields).NotTo(HaveKey("updated_at"))
	})

	It("returns accumulated validation errors instead of a balance", func() {
		result, err := balance.NewBalanceBuilder().WithAccountID("invalid-account-id").Build()

		var domainErr *fault.DomainError
		Expect(errors.As(err, &domainErr)).To(BeTrue())
		Expect(result).To(BeNil())
		Expect(domainErr.Fields).To(HaveKeyWithValue("account_id", "is not uuid"))
	})

	It("builds with explicit values", func() {
		id := uuid.NewString()
		accountUUID, err := uuid.NewV7()
		Expect(err).NotTo(HaveOccurred())
		accountID := accountUUID.String()
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
		accountID, err := uuid.NewV7()
		Expect(err).NotTo(HaveOccurred())

		result, err := balance.NewBalanceBuilder().
			WithID().
			WithAccountID(accountID.String()).
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
