//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	account "github.com/andreis3/isura-ledger-ms/internal/domain/account"
	money "github.com/andreis3/isura-ledger-ms/internal/domain/money"
	database "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	repository "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	criteria "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	uuid "github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: ACCOUNT REPOSITORY", func() {
	Context("database behavior", func() {
		It("should persist and read the account balance policy", func() {
			externalID := uuid.NewString()
			entityAccount, err := account.NewAccountBuilder().
				WithID(newIDV7()).
				WithAccountExternalID(externalID).
				WithAccountNumber(fmt.Sprintf("%d", time.Now().UnixNano())).
				WithTaxID("52998224725").
				WithStatus().
				WithType(string(account.Asset)).
				WithBalancePolicy(string(account.BalanceNonNegative)).
				WithCurrency(string(money.BRL)).
				Build()
			Expect(err).NotTo(HaveOccurred())

			repo := repository.NewAccountRepository(pool)
			txContext := database.WithTx(ctx, tx)
			Expect(repo.Save(txContext, entityAccount)).To(Succeed())

			found, err := repo.FindAccount(txContext, criteria.AccountCriteria{AccountExternalID: &externalID})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).NotTo(BeNil())
			Expect(found.BalancePolicy).To(Equal(account.BalanceNonNegative))
		})
		It("should read accounts without acquiring a pessimistic lock", func() {
			externalID, _ := insertAccountsForCommand(ctx, pool)
			blocker, err := pool.Begin(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer blocker.Rollback(ctx)
			_, err = blocker.Exec(ctx, "SELECT id FROM accounts WHERE account_external_id = $1 FOR UPDATE", externalID)
			Expect(err).NotTo(HaveOccurred())

			readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()
			found, err := repository.NewAccountRepository(pool).FindAccount(readCtx, criteria.AccountCriteria{
				AccountExternalID: &externalID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).NotTo(BeNil())
		})
	})
})
