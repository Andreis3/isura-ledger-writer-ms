//go:build unit
// +build unit

package repository

import (
	"testing"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLedgerReconciliation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ledger Reconciliation Suite")
}

var _ = Describe("ledger reconciliation replay", func() {
	const accountID = "018f5a2a-7e11-7c21-9c12-234567890abc"

	newHistoricalAccount := func() *reconciliationAccount {
		id, err := entity.NewID(accountID)
		Expect(err).NotTo(HaveOccurred())
		return &reconciliationAccount{
			account: account.Account{ID: id, AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL},
			state:   account.LedgerState{AccountID: accountID},
		}
	}

	It("reports a reconciled account with every entry counted", func() {
		historical := newHistoricalAccount()
		Expect(applyReconciliationEntry(historical, 1, transaction.Debit, 700, money.BRL)).To(Succeed())
		Expect(applyReconciliationEntry(historical, 2, transaction.Credit, 200, money.BRL)).To(Succeed())
		historical.latest = 500

		report := buildReconciliationReport(map[string]*reconciliationAccount{accountID: historical})

		Expect(report.AccountsChecked).To(Equal(1))
		Expect(report.EntriesChecked).To(Equal(2))
		Expect(report.MismatchCount).To(BeZero())
		Expect(report.Reconciled).To(BeTrue())
		Expect(report.Mismatches).To(BeEmpty())
	})

	It("reports expected and persisted balances for a mismatch", func() {
		historical := newHistoricalAccount()
		Expect(applyReconciliationEntry(historical, 1, transaction.Debit, 700, money.BRL)).To(Succeed())
		historical.latest = 650

		report := buildReconciliationReport(map[string]*reconciliationAccount{accountID: historical})

		Expect(report.Reconciled).To(BeFalse())
		Expect(report.MismatchCount).To(Equal(1))
		Expect(report.Mismatches).To(ConsistOf(ReconciliationMismatch{
			AccountID: accountID, EntriesChecked: 1, ExpectedBalance: 700,
			PersistedBalance: 650, Currency: "BRL",
		}))
	})

	It("counts accounts without entries as zero without a mismatch", func() {
		historical := newHistoricalAccount()
		report := buildReconciliationReport(map[string]*reconciliationAccount{accountID: historical})

		Expect(report.AccountsChecked).To(Equal(1))
		Expect(report.EntriesChecked).To(BeZero())
		Expect(report.Reconciled).To(BeTrue())
		Expect(report.Mismatches).To(BeEmpty())
	})

	It("returns an error for invalid accounting data", func() {
		historical := newHistoricalAccount()

		err := applyReconciliationEntry(historical, 1, "INVALID", 100, money.BRL)

		Expect(err).To(HaveOccurred())
		Expect(historical.count).To(BeZero())
		Expect(historical.state.RunningBalance).To(BeZero())
	})

	It("returns an error when entry currency differs from the account", func() {
		historical := newHistoricalAccount()

		err := applyReconciliationEntry(historical, 1, transaction.Debit, 100, money.USD)

		Expect(err).To(HaveOccurred())
		Expect(historical.count).To(BeZero())
	})

	It("returns an error when the account accounting nature is invalid", func() {
		historical := newHistoricalAccount()
		historical.account.AccountType = "INVALID"

		err := applyReconciliationEntry(historical, 1, transaction.Debit, 100, money.BRL)

		Expect(err).To(HaveOccurred())
		Expect(historical.count).To(BeZero())
	})
})

var _ = Describe("INTERNAL :: POSTGRES :: RECONCILIATION INVARIANTS",func(){
 const id="018f5a2a-7e11-7c21-9c12-234567890abc"
 newState:=func()*reconciliationAccount{
  aid,err:=entity.NewID(id)
  Expect(err).NotTo(HaveOccurred())
  return &reconciliationAccount{
   account:account.Account{ID:aid,AccountType:account.Asset,BalancePolicy:account.BalanceUnrestricted,Currency:money.BRL},
   state:account.LedgerState{AccountID:id},
  }
 }
 It("should detect a missing ledger sequence",func(){
  historical:=newState()
  Expect(applyReconciliationEntry(historical,1,transaction.Debit,100,money.BRL)).To(Succeed())
  err:=applyReconciliationEntry(historical,3,transaction.Credit,20,money.BRL)
  Expect(err).To(MatchError(ContainSubstring("non-contiguous")))
  Expect(historical.count).To(Equal(1))
  Expect(historical.state.SequenceNumber).To(Equal(int64(1)))
 })
 It("should accept an imported starting sequence and still reject a later gap",func(){
  historical:=newState()
  Expect(applyReconciliationEntry(historical,10,transaction.Debit,100,money.BRL)).To(Succeed())
  Expect(historical.state.SequenceNumber).To(Equal(int64(10)))
  Expect(applyReconciliationEntry(historical,11,transaction.Credit,20,money.BRL)).To(Succeed())
  Expect(historical.state.RunningBalance).To(Equal(int64(80)))
  err:=applyReconciliationEntry(historical,13,transaction.Debit,10,money.BRL)
  Expect(err).To(MatchError(ContainSubstring("non-contiguous")))
  Expect(historical.state.SequenceNumber).To(Equal(int64(11)))
 })
 It("should reject duplicated ledger sequence",func(){
  historical:=newState()
  Expect(applyReconciliationEntry(historical,1,transaction.Debit,100,money.BRL)).To(Succeed())
  Expect(applyReconciliationEntry(historical,1,transaction.Debit,100,money.BRL)).To(MatchError(ContainSubstring("non-contiguous")))
 })
 It("should report an intermediate corrupted running balance even if the last entry matches",func(){
  historical:=newState()
  Expect(applyReconciliationEntry(historical,1,transaction.Debit,100,money.BRL)).To(Succeed())
  compareReconciliationBalance(historical,120)
  Expect(applyReconciliationEntry(historical,2,transaction.Credit,30,money.BRL)).To(Succeed())
  compareReconciliationBalance(historical,70)
  report:=buildReconciliationReport(map[string]*reconciliationAccount{id:historical})
  Expect(report.Reconciled).To(BeFalse())
  Expect(report.MismatchCount).To(Equal(1))
  Expect(report.Mismatches[0].ExpectedBalance).To(Equal(int64(100)))
  Expect(report.Mismatches[0].PersistedBalance).To(Equal(int64(120)))
 })
})
