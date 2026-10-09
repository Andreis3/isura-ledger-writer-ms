//go:build integration

package postgres_test

import (
 "context"
 "errors"
 "time"

 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
 "github.com/jackc/pgx/v5"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

// The reconciliation runs against persisted rows AFTER a PostgreSQL backend
// dies. No accounting records are written by the reconciliation routine.
var _ = Describe("CHAOS :: POSTGRES :: POST-FAILURE LEDGER RECONCILIATION",func(){
 It("preserves the committed running balance and detects persisted corruption after a crash",func(){
  auditCtx,cancel:=context.WithTimeout(ctx,30*time.Second)
  defer cancel()
  accountID,transactionID,entryID:=insertReconciliationFixture(auditCtx,pool,1,"DEBIT",125,125)
  DeferCleanup(func(){deleteReconciliationFixture(ctx,pool,accountID,transactionID)})

  writerTx,err:=pool.Begin(auditCtx)
  Expect(err).NotTo(HaveOccurred())
  defer func(){_ = writerTx.Rollback(context.Background())}()
  var backendPID int32
  Expect(writerTx.QueryRow(auditCtx,"SELECT pg_backend_pid()").Scan(&backendPID)).To(Succeed())
  _,err=writerTx.Exec(auditCtx,"UPDATE entries SET running_balance=999 WHERE id=$1",entryID)
  Expect(err).NotTo(HaveOccurred())
  // Force a backend crash while an uncommitted modification is held.
  var killed bool
  Expect(pool.QueryRow(auditCtx,"SELECT pg_terminate_backend($1)",backendPID).Scan(&killed)).To(Succeed())
  Expect(killed).To(BeTrue())
  Expect(writerTx.Commit(auditCtx)).To(HaveOccurred())

  var balance int64
  Expect(pool.QueryRow(auditCtx,"SELECT running_balance FROM entries WHERE id=$1",entryID).Scan(&balance)).To(Succeed())
  Expect(balance).To(Equal(int64(125)),"uncommitted balance corruption must roll back")

  reconcile:=func()repository.ReconciliationReport{
   readTx,beginErr:=pool.BeginTx(auditCtx,pgx.TxOptions{IsoLevel:pgx.RepeatableRead,AccessMode:pgx.ReadOnly})
   Expect(beginErr).NotTo(HaveOccurred())
   defer func(){rollbackErr:=readTx.Rollback(auditCtx); Expect(rollbackErr==nil || errors.Is(rollbackErr,pgx.ErrTxClosed)).To(BeTrue()) }()
   report,runErr:=repository.NewLedgerReconciliation(pool).Run(database.WithTx(auditCtx,readTx))
   Expect(runErr).NotTo(HaveOccurred())
   return report
  }
  report:=reconcile()
  _,mismatch:=reconciliationMismatchFor(report,accountID)
  Expect(mismatch).To(BeFalse())

  // Deliberately persist an incorrect value: audit must detect it, proving
  // that a successful post-crash audit is not merely a no-op.
  _,err=pool.Exec(auditCtx,"UPDATE entries SET running_balance=124 WHERE id=$1",entryID)
  Expect(err).NotTo(HaveOccurred())
  DeferCleanup(func(){
   cleanupCtx,done:=context.WithTimeout(context.Background(),5*time.Second)
   defer done()
   _,resetErr:=pool.Exec(cleanupCtx,"UPDATE entries SET running_balance=125 WHERE id=$1",entryID)
   Expect(resetErr).NotTo(HaveOccurred())
  })
  report=reconcile()
  violation,found:=reconciliationMismatchFor(report,accountID)
  Expect(found).To(BeTrue())
  Expect(violation.ExpectedBalance).To(Equal(int64(125)))
  Expect(violation.PersistedBalance).To(Equal(int64(124)))
  Expect(report.Reconciled).To(BeFalse())
 })
})
