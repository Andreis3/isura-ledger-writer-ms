//go:build unit

package command_test

import (
 "context"
 "errors"

 "github.com/andreis3/isura-ledger-ms/internal/domain/account"
 "github.com/jackc/pgx/v5/pgconn"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE TRANSACTION", func() {
 Describe("#Execute", func() {
  Context("error cases", func() {
   It("should propagate a repository lookup error without creating any entries or outbox event", func() {
    // Arrange.
    failure := errors.New("transaction lookup failed")
    transactions := &transactionRepository{findErr: failure}
    outboxes := &outboxRepository{}
    accounts := newAccountRepository()
    sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

    // Act.
    result, err := sut.Execute(context.Background(), validInput())

    // Assert.
    Expect(result).To(BeNil())
    Expect(errors.Is(err, failure)).To(BeTrue())
    Expect(transactions.saveCalls).To(BeZero())
    Expect(outboxes.saved).To(BeNil())
    Expect(accounts.findCalls).To(BeEmpty())
   })

   It("should reject a blocked account before persisting a transaction", func() {
    // Arrange.
    accounts := newAccountRepository()
    accounts.accounts[creditExternalID].Status = account.StatusBlocked
    transactions := &transactionRepository{}
    outboxes := &outboxRepository{}
    sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

    // Act.
    result, err := sut.Execute(context.Background(), validInput())

    // Assert.
    Expect(result).To(BeNil())
    Expect(err).To(HaveOccurred())
    Expect(transactions.saveCalls).To(BeZero())
    Expect(outboxes.saved).To(BeNil())
   })

   It("should return an error if an idempotency race is not followed by a persisted transaction", func() {
    // Arrange.
    collision := &pgconn.PgError{Code: "23505", ConstraintName: "idx_transactions_idempotency_key"}
    transactions := &transactionRepository{saveErrors: []error{collision}}
    outboxes := &outboxRepository{}
    sut := newCommand(newAccountRepository(), transactions, outboxes, &unitOfWork{})

    // Act.
    result, err := sut.Execute(context.Background(), validInput())

    // Assert.
    Expect(result).To(BeNil())
    Expect(errors.Is(err, collision)).To(BeTrue())
    Expect(transactions.saveCalls).To(Equal(1))
    Expect(outboxes.saved).To(BeNil())
   })

  })
 })
})
