//go:build integration

package postgres_test

import (
 "context"
 "errors"
 "fmt"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

// This test kills only the backend of a dedicated connection within the
// disposable Testcontainers PostgreSQL, never the shared suite pool.
var _ = Describe("CHAOS :: POSTGRES :: CONNECTION LOSS",func(){
 It("should roll back uncommitted financial records after its backend is terminated",func(){
  testCtx,cancel:=context.WithTimeout(ctx,20*time.Second)
  defer cancel()
  dbTx,err:=pool.BeginTx(testCtx,pgx.TxOptions{IsoLevel:pgx.Serializable})
  Expect(err).NotTo(HaveOccurred())
  defer func(){_ = dbTx.Rollback(context.Background())}()

  key:="chaos-pg-"+uuid.NewString()
  id:=uuid.NewString()
  var pid int32
  Expect(dbTx.QueryRow(testCtx,"SELECT pg_backend_pid()").Scan(&pid)).To(Succeed())
  _,err=dbTx.Exec(testCtx,`
   INSERT INTO transactions
     (id,idempotency_key,request_fingerprint,status,operation,amount,currency,created_at,updated_at)
   VALUES ($1,$2,$3,'COMPLETED','TRANSFER',10,'BRL',now(),now())
  `,id,key,"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
  Expect(err).NotTo(HaveOccurred())

  // Kill the specific PostgreSQL session from a DIFFERENT connection.
  var terminated bool
  Expect(pool.QueryRow(testCtx,"SELECT pg_terminate_backend($1)",pid).Scan(&terminated)).To(Succeed())
  Expect(terminated).To(BeTrue())
  Expect(dbTx.Commit(testCtx)).To(HaveOccurred())

  var count int
  Expect(pool.QueryRow(testCtx,"SELECT count(*) FROM transactions WHERE idempotency_key=$1",key).Scan(&count)).To(Succeed())
  Expect(count).To(BeZero(),"an uncommitted transaction must not survive connection loss")

  // Verify normal transactions work after the connection is replaced.
  recoveryKey:="chaos-ok-"+uuid.NewString()
  _,err=pool.Exec(testCtx,`
   INSERT INTO transactions
     (id,idempotency_key,request_fingerprint,status,operation,amount,currency,created_at,updated_at)
   VALUES ($1,$2,$3,'COMPLETED','TRANSFER',10,'BRL',now(),now())
  `,uuid.NewString(),recoveryKey,"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,cleanupCancel:=context.WithTimeout(context.Background(),5*time.Second)
   defer cleanupCancel()
   _,cleanupErr:=pool.Exec(cleanupCtx,"DELETE FROM transactions WHERE idempotency_key=$1",recoveryKey)
   Expect(cleanupErr).NotTo(HaveOccurred())
  })
  Expect(pool.QueryRow(testCtx,"SELECT count(*) FROM transactions WHERE idempotency_key=$1",recoveryKey).Scan(&count)).To(Succeed())
  Expect(count).To(Equal(1),fmt.Sprintf("database did not recover: %v",errors.New("missing committed row")))
 })
})
