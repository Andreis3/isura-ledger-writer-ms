//go:build integration

package postgres_test

import (
 "time"

 "github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTEGRATION :: POSTGRES :: OUTBOX CRASH RECOVERY",func(){
 It("should reclaim an unacknowledged outbox publish without changing event identity",func(){
  repo:=repository.NewOutBoxRepository(pool)
  txCtx:=database.WithTx(ctx,tx)
  item:=newOutbox("crash-recovery-test")
  Expect(repo.Save(txCtx,item)).To(Succeed())

  first,err:=repo.ClaimPending(txCtx,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(first).To(HaveLen(1))
  Expect(first[0].ID).To(Equal(item.ID))
  Expect(first[0].Attempts).To(Equal(1))
  Expect(first[0].Status).To(Equal(outbox.Failed))

  // Simulate process termination after claim but before recording NATS ACK.
  // A new relay instance uses the persisted FAILED state after RetryAfter.
  again,err:=repo.ClaimPending(txCtx,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(again).To(HaveLen(1))
  Expect(again[0].ID).To(Equal(item.ID))
  Expect(again[0].Attempts).To(Equal(2))
  Expect(again[0].Payload).To(Equal(first[0].Payload))

  publishedAt:=time.Now()
  Expect(repo.UpdateOutboxData(txCtx,item.ID,outbox.UpdateOutboxData{
   Status:outbox.Success,
   Attempts:again[0].Attempts,
   LastAttemptAt:again[0].LastAttemptAt,
   PublishedAt:&publishedAt,
  })).To(Succeed())
  noMore,err:=repo.ClaimPending(txCtx,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(noMore).To(BeEmpty())
 })
 It("should stop reclaiming events when retry budget has been exhausted",func(){
  repo:=repository.NewOutBoxRepository(pool)
  txCtx:=database.WithTx(ctx,tx)
  item:=newOutbox("exhausted-recovery-test")
  Expect(repo.Save(txCtx,item)).To(Succeed())
  for attempt:=1;attempt<=outbox.MaxAttempts;attempt++{
   claimed,err:=repo.ClaimPending(txCtx,1,outbox.MaxAttempts,0)
   Expect(err).NotTo(HaveOccurred())
   Expect(claimed).To(HaveLen(1))
   Expect(claimed[0].Attempts).To(Equal(attempt))
  }
  exhausted,err:=repo.ClaimPending(txCtx,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(exhausted).To(BeEmpty())
 })
})
