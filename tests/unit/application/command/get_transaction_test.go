//go:build unit

package command_test

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type getTransactionRepositoryFake struct {
	criteria transaction.TransactionCriteria
	result   *transaction.Transaction
	err      error
}

func (f *getTransactionRepositoryFake) Save(context.Context, *transaction.Transaction) error {
	return nil
}
func (f *getTransactionRepositoryFake) Find(_ context.Context, criteria transaction.TransactionCriteria) (*transaction.Transaction, error) {
	f.criteria = criteria
	return f.result, f.err
}
func (f *getTransactionRepositoryFake) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	return 0, 0, nil
}
func (f *getTransactionRepositoryFake) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	return false, nil
}

var _ transaction.Repository = (*getTransactionRepositoryFake)(nil)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: GET TRANSACTION", func() {
	Describe("#Execute", func() {
		Context("success cases", func() {
			It("should fetch the transaction with all entries", func() {
				// Arrange
				repository := &getTransactionRepositoryFake{}
				sut := command.NewGetTransaction(repository)
				id := "transaction-id"

				// Act
				result, err := sut.Execute(context.Background(), id)

				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(repository.criteria.ID).NotTo(BeNil())
				Expect(*repository.criteria.ID).To(Equal(id))
				Expect(repository.criteria.WithEntries).To(BeTrue())
			})
		})

		Context("error cases", func() {
			It("should map a missing transaction to the public not found fault", func() {
				// Arrange
				repository := &getTransactionRepositoryFake{err: transaction.ErrTransactionNotFound}
				sut := command.NewGetTransaction(repository)

				// Act
				_, err := sut.Execute(context.Background(), "missing")

				// Assert
				Expect(err).To(BeIdenticalTo(fault.ErrTransactionNotFound))
			})
		})
	})
})
