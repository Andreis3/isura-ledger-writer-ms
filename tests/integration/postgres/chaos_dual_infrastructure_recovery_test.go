//go:build integration

package postgres_test

import (
 "context"
 "fmt"
 "os/exec"
 "path/filepath"
 "runtime"
 "time"

 "github.com/andreis3/isura-ledger-ms/internal/application"
 "github.com/andreis3/isura-ledger-ms/internal/application/command"
 "github.com/andreis3/isura-ledger-ms/internal/application/dto"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
 "github.com/andreis3/isura-ledger-ms/internal/infra/configs"
 infranats "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
 adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
 natsgo "github.com/nats-io/nats.go"
 "github.com/nats-io/nats.go/jetstream"
 "github.com/testcontainers/testcontainers-go"
 "github.com/testcontainers/testcontainers-go/modules/postgres"
 "github.com/testcontainers/testcontainers-go/wait"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("CHAOS :: SIMULTANEOUS POSTGRES AND NATS OUTAGE",func(){
 It("preserves a committed double-entry transfer and delivers its outbox exactly once after both services recover",func(){
  runCtx,cancel:=context.WithTimeout(context.Background(),120*time.Second)
  defer cancel()

  // A dedicated database is essential: the Ginkgo suite uses a different
  // PostgreSQL container and its own rollback-only transaction.
  dbContainer,err:=postgres.Run(runCtx,"postgres:18-alpine",
   postgres.WithDatabase("isura_dual_chaos"),
   postgres.WithUsername("admin"),postgres.WithPassword("admin"),
   postgres.BasicWaitStrategies(),
  )
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,done:=context.WithTimeout(context.Background(),20*time.Second)
   defer done()
   Expect(testcontainers.TerminateContainer(dbContainer)).To(Succeed())
  })
  dbURL,err:=dbContainer.ConnectionString(runCtx,"sslmode=disable")
  Expect(err).NotTo(HaveOccurred())
  _,sourceFile,_,ok:=runtime.Caller(0)
  Expect(ok).To(BeTrue())
  dbDir:=filepath.Clean(filepath.Join(filepath.Dir(sourceFile),"../../../db"))
  migrate:=exec.CommandContext(runCtx,"atlas","schema","apply","--auto-approve","--url",dbURL,"--to","file://"+dbDir)
  output,err:=migrate.CombinedOutput()
  Expect(err).NotTo(HaveOccurred(),string(output))

  db,err:=pgxpool.New(runCtx,dbURL)
  Expect(err).NotTo(HaveOccurred())
  defer db.Close()
  Expect(db.Ping(runCtx)).To(Succeed())

  broker,err:=testcontainers.GenericContainer(runCtx,testcontainers.GenericContainerRequest{
   Started:true,
   ContainerRequest:testcontainers.ContainerRequest{
    Image:"nats:2.11-alpine",Cmd:[]string{"-js","-sd","/tmp/jetstream"},
    ExposedPorts:[]string{"4222/tcp"},WaitingFor:wait.ForListeningPort("4222/tcp"),
   },
  })
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,done:=context.WithTimeout(context.Background(),20*time.Second)
   defer done()
   Expect(broker.Terminate(cleanupCtx)).To(Succeed())
  })
  natsURL:=func()string{
   host,hostErr:=broker.Host(runCtx)
   Expect(hostErr).NotTo(HaveOccurred())
   port,portErr:=broker.MappedPort(runCtx,"4222/tcp")
   Expect(portErr).NotTo(HaveOccurred())
   return fmt.Sprintf("nats://%s:%s",host,port.Port())
  }
  nc,err:=natsgo.Connect(natsURL(),natsgo.Timeout(3*time.Second),natsgo.NoReconnect())
  Expect(err).NotTo(HaveOccurred())
  js,err:=jetstream.New(nc)
  Expect(err).NotTo(HaveOccurred())
  _,err=js.CreateStream(runCtx,jetstream.StreamConfig{
   Name:"CHAOS_DUAL",Subjects:[]string{"ledger.transaction.created"},Duplicates:2*time.Minute,
  })
  Expect(err).NotTo(HaveOccurred())

  debit,credit:=insertAccountsForCommand(runCtx,db)
  key:="dual-"+uuid.NewString()
  amount:=int64(10)
  currency:=string(money.BRL)
  operation:=string(transaction.OperationTransfer)
  input:=dto.CreateTransactionInput{
   IdempotencyKey:&key,DebitAccountID:&debit,CreditAccountID:&credit,
   Amount:&amount,Currency:&currency,Operation:&operation,
  }
  create:=func(p *pgxpool.Pool)*command.CreateTransaction{
   return command.NewCreateTransaction(
    uow.NewUnitOfWork(p),repository.NewAccountRepository(p),
    repository.NewTransactionRepository(p),repository.NewOutBoxRepository(p),
    adaptermocks.SilentTracerMock{},adaptermocks.SilentLoggerMock{},
    adaptermocks.SilentMetricsMock{},application.DefaultMaxTransactionEntries,
   )
  }
  created,err:=create(db).Execute(runCtx,input)
  Expect(err).NotTo(HaveOccurred())
  Expect(created.IdempotentReplay).To(BeFalse())
  transactionID:=*created.TransactionID
  var entryCount,outboxCount int
  var eventID string
  Expect(db.QueryRow(runCtx,"SELECT count(*) FROM entries WHERE transaction_id=$1",transactionID).Scan(&entryCount)).To(Succeed())
  Expect(entryCount).To(Equal(2))
  Expect(db.QueryRow(runCtx,"SELECT count(*) FROM outbox_events WHERE aggregate_id=$1",transactionID).Scan(&outboxCount)).To(Succeed())
  Expect(outboxCount).To(Equal(1))
  Expect(db.QueryRow(runCtx,"SELECT id FROM outbox_events WHERE aggregate_id=$1",transactionID).Scan(&eventID)).To(Succeed())

  // Outage begins after the atomic transfer + outbox commit, before publish.
  nc.Close()
  db.Close()
  Expect(broker.Stop(runCtx,nil)).To(Succeed())
  Expect(dbContainer.Stop(runCtx,nil)).To(Succeed())
  Expect(dbContainer.Start(runCtx)).To(Succeed())
  Expect(broker.Start(runCtx)).To(Succeed())

  // Docker may reassign published ports after Start.
  var recoveredDB *pgxpool.Pool
  Eventually(func(g Gomega){
   freshURL,urlErr:=dbContainer.ConnectionString(runCtx,"sslmode=disable")
   g.Expect(urlErr).NotTo(HaveOccurred())
   candidate,openErr:=pgxpool.New(runCtx,freshURL)
   g.Expect(openErr).NotTo(HaveOccurred())
   pingCtx,pingCancel:=context.WithTimeout(runCtx,2*time.Second)
   pingErr:=candidate.Ping(pingCtx)
   pingCancel()
   if pingErr!=nil {candidate.Close()}
   g.Expect(pingErr).NotTo(HaveOccurred())
   recoveredDB=candidate
  },20*time.Second,500*time.Millisecond).Should(Succeed())
  defer recoveredDB.Close()

  var recoveredNC *natsgo.Conn
  Eventually(func(g Gomega){
   candidate,connectErr:=natsgo.Connect(natsURL(),natsgo.Timeout(time.Second),natsgo.NoReconnect())
   g.Expect(connectErr).NotTo(HaveOccurred())
   recoveredNC=candidate
  },20*time.Second,300*time.Millisecond).Should(Succeed())
  defer recoveredNC.Close()
  recoveredJS,err:=jetstream.New(recoveredNC)
  Expect(err).NotTo(HaveOccurred())

  replay,err:=create(recoveredDB).Execute(runCtx,input)
  Expect(err).NotTo(HaveOccurred())
  Expect(replay.IdempotentReplay).To(BeTrue())
  Expect(*replay.TransactionID).To(Equal(transactionID))

  relay:=infranats.NewOutboxRelay(
   repository.NewOutBoxRepository(recoveredDB),recoveredJS,
   adaptermocks.SilentTracerMock{},adaptermocks.SilentLoggerMock{},
   adaptermocks.SilentMetricsMock{},
   configs.OutboxRelay{BatchSize:1000,MaxAttempts:outbox.MaxAttempts,RetryAfter:time.Millisecond,MaxWorkers:2},
  )
  Expect(relay.PublishBatch(runCtx)).To(Succeed())
  Expect(relay.PublishBatch(runCtx)).To(Succeed(),"an already published event must not be sent again")

  var transactions int
  var status string
  var publishedAt *time.Time
  Expect(recoveredDB.QueryRow(runCtx,"SELECT count(*) FROM transactions WHERE idempotency_key=$1",key).Scan(&transactions)).To(Succeed())
  Expect(recoveredDB.QueryRow(runCtx,"SELECT count(*) FROM entries WHERE transaction_id=$1",transactionID).Scan(&entryCount)).To(Succeed())
  Expect(recoveredDB.QueryRow(runCtx,"SELECT count(*) FROM outbox_events WHERE aggregate_id=$1",transactionID).Scan(&outboxCount)).To(Succeed())
  Expect(recoveredDB.QueryRow(runCtx,"SELECT status,published_at FROM outbox_events WHERE id=$1",eventID).Scan(&status,&publishedAt)).To(Succeed())
  Expect(transactions).To(Equal(1))
  Expect(entryCount).To(Equal(2))
  Expect(outboxCount).To(Equal(1))
  Expect(status).To(Equal(string(outbox.Success)))
  Expect(publishedAt).NotTo(BeNil())
  stream,err:=recoveredJS.Stream(runCtx,"CHAOS_DUAL")
  Expect(err).NotTo(HaveOccurred())
  info,err:=stream.Info(runCtx)
  Expect(err).NotTo(HaveOccurred())
  Expect(info.State.Msgs).To(Equal(uint64(1)))
 })
})
