//go:build unit

package command_test

import (
 "context"
 "encoding/json"
 "errors"

 "github.com/andreis3/isura-ledger-ms/internal/application/dto"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE TRANSACTION",func(){
 Describe("#saveCreatedEvent",func(){
  Context("success cases",func(){
   It("should serialize a canonical multi-entry event with the persisted outbox event ID",func(){
    // Arrange.
    input:=validInput()
    input.DebitAccountID=nil
    input.CreditAccountID=nil
    input.Currency=nil
    input.Entries=[]dto.EntryInput{
     {AccountID:debitExternalID,Direction:string(transaction.Debit),Amount:150000,Currency:string(money.BRL)},
     {AccountID:creditExternalID,Direction:string(transaction.Credit),Amount:100000,Currency:string(money.BRL)},
     {AccountID:creditExternalID,Direction:string(transaction.Credit),Amount:50000,Currency:string(money.BRL)},
    }
    outboxes:=&outboxRepository{}
    transactions:=&transactionRepository{}
    sut:=newCommand(newAccountRepository(),transactions,outboxes,&unitOfWork{})

    // Act.
    output,err:=sut.Execute(context.Background(),input)

    // Assert.
    Expect(err).NotTo(HaveOccurred())
    Expect(output).NotTo(BeNil())
    Expect(outboxes.saved).NotTo(BeNil())
    var event transaction.TransactionCreated
    Expect(json.Unmarshal(outboxes.saved.Payload,&event)).To(Succeed())
    Expect(event.EventID).To(Equal(outboxes.saved.ID.String()))
    Expect(event.TransactionID).To(Equal(*output.TransactionID))
    Expect(event.IdempotencyKey).To(Equal(*input.IdempotencyKey))
    Expect(event.Status).To(Equal(string(transaction.Completed)))
    Expect(event.Amount).To(Equal(int64(150000)))
    Expect(event.Currency).To(Equal(string(money.BRL)))
    Expect(event.Entries).To(HaveLen(3))
    for position,entry:=range event.Entries {
     Expect(entry.Position).To(Equal(int64(position)))
     Expect(entry.AccountID).To(Equal(transactions.saved.Entries[position].AccountID))
     Expect(entry.Amount).To(Equal(input.Entries[position].Amount))
     Expect(entry.Direction).To(Equal(transaction.Direction(input.Entries[position].Direction)))
    }
   })
  })
  Context("error cases",func(){
   It("should propagate the outbox repository failure without reporting a confirmed output",func(){
    // Arrange.
    failure:=errors.New("outbox persistence unavailable")
    outboxes:=&outboxRepository{saveErr:failure}
    transactions:=&transactionRepository{}
    sut:=newCommand(newAccountRepository(),transactions,outboxes,&unitOfWork{})

    // Act.
    output,err:=sut.Execute(context.Background(),validInput())

    // Assert.
    Expect(output).To(BeNil())
    Expect(errors.Is(err,failure)).To(BeTrue())
    Expect(outboxes.saved).To(BeNil())
    Expect(transactions.saveCalls).To(Equal(1))
   })
  })
 })
})
