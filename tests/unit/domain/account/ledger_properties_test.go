//go:build unit

package account_test

import (
 "math"
 "math/rand"
 "testing"

 "github.com/andreis3/isura-ledger-ms/internal/domain/account"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

// Deterministic model-based property test: independent arithmetic oracle for
// thousands of ledger histories. Seed is fixed to reproduce counterexamples.
func TestLedgerStateMachineProperties(t *testing.T) {
 rng:=rand.New(rand.NewSource(20261009))
 types:=[]struct{kind account.Type;debitIncreases bool}{
  {account.Asset,true},{account.Expense,true},
  {account.Liability,false},{account.Equity,false},{account.Revenue,false},
 }
 for _,kind:=range types{
  for history:=0;history<500;history++{
   current:=account.LedgerState{}
   expected:=int64(0)
   a:=account.Account{AccountType:kind.kind,BalancePolicy:account.BalanceUnrestricted,Currency:money.BRL}
   for step:=int64(1);step<=30;step++{
    amount:=int64(rng.Intn(1000000)+1)
    direction:=transaction.Debit
    if rng.Intn(2)==0 {direction=transaction.Credit}
    delta:=amount
    if (direction==transaction.Debit)!=kind.debitIncreases {delta=-amount}
    value,err:=money.NewMoney(amount,money.BRL)
    if err!=nil {t.Fatalf("history %d step %d: %v",history,step,err)}
    next,err:=a.ApplyEntry(current,direction,value)
    if err!=nil {t.Fatalf("history %d step %d: %v",history,step,err)}
    expected+=delta
    if next.SequenceNumber!=step || next.RunningBalance!=expected {
     t.Fatalf("account type %s history %d step %d: got (%d,%d), expected (%d,%d)",kind.kind,history,step,next.SequenceNumber,next.RunningBalance,step,expected)
    }
    // ApplyEntry must never mutate the previously supplied value.
    if current.SequenceNumber!=step-1 {t.Fatal("previous ledger state mutated")}
    current=next
   }
  }
 }
}

func TestLedgerOverflowProperty(t *testing.T) {
 a:=account.Account{AccountType:account.Asset,BalancePolicy:account.BalanceUnrestricted,Currency:money.BRL}
 v,err:=money.NewMoney(1,money.BRL)
 if err!=nil {t.Fatal(err)}
 for _,start:=range []int64{math.MaxInt64, math.MinInt64}{
  dir:=transaction.Debit
  if start==math.MinInt64 {dir=transaction.Credit}
  state:=account.LedgerState{SequenceNumber:1,RunningBalance:start}
  next,err:=a.ApplyEntry(state,dir,v)
  if err==nil || next!=state {t.Fatalf("overflow must preserve state: %+v, %v",next,err)}
 }
}
