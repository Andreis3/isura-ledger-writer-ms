//go:build integration

package postgres_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: BALANCE REPOSITORY", func() {
	Describe("#Save and #Find", func() {
		Context("success cases", func() {
			It("should persist and retrieve a balance by its account", func() {
				// Arrange (Given)
				accountID := insertAccountV7(ctx, tx)
				createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				balanceEntity, err := balance.NewBalanceBuilder().
					WithID().
					WithAccountID(accountID).
					WithAmount(7250, money.BRL).
					WithCreatedAt(createdAt).
					WithUpdatedAt(createdAt).
					Build()
				Expect(err).NotTo(HaveOccurred())
				repo := repository.NewBalanceRepository(pool)
				txContext := database.WithTx(ctx, tx)

				// Act (When)
				err = repo.Save(txContext, balanceEntity)
				Expect(err).NotTo(HaveOccurred())
				found, err := repo.Find(txContext, criteria.BalanceCriteria{AccountID: &accountID})

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).NotTo(BeNil())
				Expect(found.ID()).To(Equal(balanceEntity.ID()))
				Expect(found.AccountID()).To(Equal(accountID))
				Expect(found.Amount()).To(Equal(balanceEntity.Amount()))
				Expect(found.CreatedAT()).To(BeTemporally("==", createdAt))
			})
		})

		Context("error cases", func() {
			It("should return no balance when the account has no persisted balance", func() {
				// Arrange (Given)
				accountID := insertAccountV7(ctx, tx)
				repo := repository.NewBalanceRepository(pool)
				txContext := database.WithTx(ctx, tx)

				// Act (When)
				found, err := repo.Find(txContext, criteria.BalanceCriteria{AccountID: &accountID})

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeNil())
			})
		})
	})
})
