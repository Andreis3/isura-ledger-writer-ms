//go:build unit

package command_test

import (
 "context"
 "errors"

 "github.com/andreis3/isura-ledger-ms/internal/application/command"
 "github.com/andreis3/isura-ledger-ms/internal/domain/fault"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

type indeterminateCommitUOW struct{ calls int }
func (u *indeterminateCommitUOW) WithTransaction(ctx context.Context, fn func(context.Context)error)error{return u.WithRetryableTransaction(ctx,fn)}
func (u *indeterminateCommitUOW) WithRetryableTransaction(_ context.Context,_ func(context.Context)error)error{
 u.calls++
 return errors.Join(errors.New("connection lost while committing"),fault.ErrCommitOutcomeUnknown)
}

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: UNCERTAIN COMMIT",func(){
 Describe("#Execute",func(){
  It("should recover a committed result by idempotency key without writing again",func(){
   input:=validInput()
   persisted,err:=input.CreateTransactionFacade()
   Expect(err).NotTo(HaveOccurred())
   Expect(persisted.Complete()).To(Succeed())
   transactions:=&transactionRepository{existing:persisted}
   uow:=&indeterminateCommitUOW{}
   sut:=newCommand(newAccountRepository(),transactions,&outboxRepository{},uow)
   result,err:=sut.Execute(context.Background(),input)
   Expect(err).NotTo(HaveOccurred())
   Expect(result.TransactionID).To(Equal(new(persisted.ID.String())))
   Expect(result.Status).To(Equal(string(transaction.Completed)))
   Expect(result.IdempotentReplay).To(BeTrue())
   Expect(transactions.saveCalls).To(BeZero())
   Expect(transactions.findCalls).To(Equal(1))
   Expect(uow.calls).To(Equal(1))
  })
  It("should preserve uncertain outcome when the committed row cannot be confirmed",func(){
   transactions:=&transactionRepository{}
   uow:=&indeterminateCommitUOW{}
   sut:=newCommand(newAccountRepository(),transactions,&outboxRepository{},uow)
   result,err:=sut.Execute(context.Background(),validInput())
   Expect(result).To(BeNil())
   Expect(errors.Is(err,fault.ErrCommitOutcomeUnknown)).To(BeTrue())
   Expect(transactions.saveCalls).To(BeZero())
   Expect(uow.calls).To(Equal(1))
  })
  It("should reject mismatched intent while resolving an unknown commit",func(){
   input:=validInput()
   existing,err:=input.CreateTransactionFacade()
   Expect(err).NotTo(HaveOccurred())
   existing.Fingerprint="different"
   transactions:=&transactionRepository{existing:existing}
   sut:=newCommand(newAccountRepository(),transactions,&outboxRepository{},&indeterminateCommitUOW{})
   result,err:=sut.Execute(context.Background(),input)
   Expect(result).To(BeNil())
   var domainErr *fault.DomainError
   Expect(errors.As(err,&domainErr)).To(BeTrue())
   Expect(domainErr.Code).To(Equal(fault.CodeDuplicateTransaction))
   Expect(transactions.saveCalls).To(BeZero())
  })
 })
})
