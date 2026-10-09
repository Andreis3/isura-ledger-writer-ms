//go:build integration

package postgres_test

import (
 "context"
 "fmt"
 "time"

 "github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
 natsgo "github.com/nats-io/nats.go"
 "github.com/nats-io/nats.go/jetstream"
 "github.com/testcontainers/testcontainers-go"
 "github.com/testcontainers/testcontainers-go/wait"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("CHAOS :: OUTBOX ACK LOSS AND WORKER RESTART",func(){
 It("should deduplicate a redelivered event after NATS acknowledges but before PostgreSQL marks it published",func(){
  chaosCtx,cancel:=context.WithTimeout(ctx,60*time.Second)
  defer cancel()
  broker,err:=testcontainers.GenericContainer(chaosCtx,testcontainers.GenericContainerRequest{
   Started:true,
   ContainerRequest:testcontainers.ContainerRequest{
    Image:"nats:2.11-alpine",
    Cmd:[]string{"-js"},
    ExposedPorts:[]string{"4222/tcp"},
    WaitingFor:wait.ForListeningPort("4222/tcp"),
   },
  })
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,done:=context.WithTimeout(context.Background(),20*time.Second)
   defer done()
   Expect(broker.Terminate(cleanupCtx)).To(Succeed())
  })
  host,err:=broker.Host(chaosCtx)
  Expect(err).NotTo(HaveOccurred())
  port,err:=broker.MappedPort(chaosCtx,"4222/tcp")
  Expect(err).NotTo(HaveOccurred())
  nc,err:=natsgo.Connect(fmt.Sprintf("nats://%s:%s",host,port.Port()),natsgo.Timeout(3*time.Second))
  Expect(err).NotTo(HaveOccurred())
  defer nc.Close()
  js,err:=jetstream.New(nc)
  Expect(err).NotTo(HaveOccurred())
  _,err=js.CreateStream(chaosCtx,jetstream.StreamConfig{
   Name:"CHAOS_OUTBOX",
   Subjects:[]string{"ledger.transaction.created"},
   Duplicates:2*time.Minute,
  })
  Expect(err).NotTo(HaveOccurred())

  // Suite BeforeEach gives us a rollback-only transaction, leaving every
  // pre-existing event unchanged. Simulate a crashed relay by creating a NEW
  // repository instance after the first publish acknowledgement.
  txContext:=database.WithTx(chaosCtx,tx)
  _,err=tx.Exec(chaosCtx,"DELETE FROM outbox_events")
  Expect(err).NotTo(HaveOccurred())
  item:=newOutbox("chaos-outbox-restart")
  Expect(repository.NewOutBoxRepository(pool).Save(txContext,item)).To(Succeed())

  first,err:=repository.NewOutBoxRepository(pool).ClaimPending(txContext,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(first).To(HaveLen(1))
  Expect(first[0].ID).To(Equal(item.ID))
  msg:=&natsgo.Msg{Subject:string(item.EventType),Data:item.Payload,Header:natsgo.Header{}}
  msg.Header.Set("Nats-Msg-Id",item.ID.String())
  ack,err:=js.PublishMsg(chaosCtx,msg)
  Expect(err).NotTo(HaveOccurred())
  Expect(ack.Duplicate).To(BeFalse())

  // Lost database acknowledgement: event is still FAILED and retryable.
  var status string
  Expect(tx.QueryRow(chaosCtx,"SELECT status FROM outbox_events WHERE id=$1",item.ID.String()).Scan(&status)).To(Succeed())
  Expect(status).To(Equal(string(outbox.Failed)))

  restartedRepo:=repository.NewOutBoxRepository(pool)
  second,err:=restartedRepo.ClaimPending(txContext,1,outbox.MaxAttempts,0)
  Expect(err).NotTo(HaveOccurred())
  Expect(second).To(HaveLen(1))
  Expect(second[0].ID).To(Equal(item.ID))
  Expect(second[0].Attempts).To(Equal(2))
  retryAck,err:=js.PublishMsg(chaosCtx,msg)
  Expect(err).NotTo(HaveOccurred())
  Expect(retryAck.Duplicate).To(BeTrue(),"JetStream should deduplicate a repeated Nats-Msg-Id")

  now:=time.Now()
  Expect(restartedRepo.UpdateOutboxData(txContext,item.ID,outbox.UpdateOutboxData{
   Status:outbox.Success,
   Attempts:second[0].Attempts,
   LastAttemptAt:second[0].LastAttemptAt,
   PublishedAt:&now,
  })).To(Succeed())
  Expect(tx.QueryRow(chaosCtx,"SELECT status FROM outbox_events WHERE id=$1",item.ID.String()).Scan(&status)).To(Succeed())
  Expect(status).To(Equal(string(outbox.Success)))
  stream,err:=js.Stream(chaosCtx,"CHAOS_OUTBOX")
  Expect(err).NotTo(HaveOccurred())
  info,err:=stream.Info(chaosCtx)
  Expect(err).NotTo(HaveOccurred())
  Expect(info.State.Msgs).To(Equal(uint64(1)))
 })
})
