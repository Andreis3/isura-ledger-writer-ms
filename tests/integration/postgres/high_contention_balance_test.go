//go:build integration

package postgres_test

import (
 "errors"
 "fmt"
 "sync"
 "time"

 "github.com/andreis3/isura-ledger-ms/internal/domain/fault"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
 "github.com/google/uuid"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTEGRATION :: POSTGRES :: HIGH CONTENTION BALANCE",func(){
 It("should never overdraw an account under 100 competing debit requests",func(){
  debit,credit:=insertFundedAccountsForCommand(ctx,pool,100,1)
  command:=newIntegrationCreateTransaction(pool)
  prefix:="soak-"+uuid.NewString()+"-"
  const requestCount=100
  const maxWorkers=10
  results:=make(chan error,requestCount)
  permits:=make(chan struct{},maxWorkers)
  start:=make(chan struct{})
  var wg sync.WaitGroup
  for index:=0;index<requestCount;index++{
   wg.Add(1)
   go func(index int){
    defer wg.Done()
    permits<-struct{}{}
    defer func(){<-permits}()
    <-start
    key:=fmt.Sprintf("%s%d",prefix,index)
    for attempt:=0;attempt<20;attempt++{
     result:=executeIntegrationTransaction(ctx,command,debit,credit,key,10)
     if result.err==nil || !errors.Is(result.err,uow.ErrMaxRetriesExceeded){
      results<-result.err
      return
     }
     time.Sleep(5*time.Millisecond)
    }
    results<-fmt.Errorf("contention retries exhausted for request %d",index)
   }(index)
  }
  close(start)
  wg.Wait()
  close(results)

  successes,denied:=0,0
  for err:=range results{
   switch {
   case err==nil:
    successes++
   case errors.Is(err,fault.ErrInsufficientBalance):
    denied++
   default:
    Fail(fmt.Sprintf("unexpected transaction result: %v",err))
   }
  }
  Expect(successes).To(Equal(10))
  Expect(denied).To(Equal(90))

  var persisted int
  Expect(pool.QueryRow(ctx,"SELECT count(*) FROM transactions WHERE idempotency_key LIKE $1",prefix+"%").Scan(&persisted)).To(Succeed())
  Expect(persisted).To(Equal(10))

  var lastBalance int64
  Expect(pool.QueryRow(ctx,`
   SELECT e.running_balance FROM entries e
   JOIN accounts a ON e.account_id=a.id
   WHERE a.account_external_id=$1
   ORDER BY e.sequence_number DESC LIMIT 1
  `,debit).Scan(&lastBalance)).To(Succeed())
  Expect(lastBalance).To(Equal(int64(0)))
 })
})
