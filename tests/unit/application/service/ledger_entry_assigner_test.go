//go:build unit
// +build unit

package service_test

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/application/service"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: SERVICE :: LEDGER ENTRY ASSIGNER", func() {
	Describe("#AssignLedgerEntries", func() {
		Context("success cases", func() {
			It("should assign positions and progressive states in input order", func() {
				ctx := context.Background()
				debitAccount := newLedgerAccount("019ff448-c43d-70d3-83c7-dfa0674469b7", account.Asset)
				creditAccount := newLedgerAccount("019ff448-c43d-70d3-83c7-dfa0674469b8", account.Asset)
				repository := &transactionRepositoryFake{states: map[string]account.LedgerState{
					debitAccount.ID.String():  {AccountID: debitAccount.ID.String()},
					creditAccount.ID.String(): {AccountID: creditAccount.ID.String()},
				}}
				entityTransaction := newMultiEntryTransaction(debitAccount.ID.String(), creditAccount.ID.String())

				// Act (When).
				err := service.AssignLedgerEntries(ctx, repository, entityTransaction, debitAccount, creditAccount)

				// Assert (Then).
				Expect(err).NotTo(HaveOccurred())
				Expect(repository.findOrder).To(Equal([]string{debitAccount.ID.String(), creditAccount.ID.String()}))
				Expect(entryPositions(entityTransaction.Entries)).To(Equal([]int64{0, 1, 2, 3}))
				Expect(entrySequences(entityTransaction.Entries)).To(Equal([]int64{1, 1, 2, 2}))
				Expect(entryBalances(entityTransaction.Entries)).To(Equal([]int64{1000, -600, 1500, -1500}))
			})
		})

		Context("error cases", func() {
			It("should stop before assigning an entry that violates the account balance policy", func() {
				ctx := context.Background()
				otherAccount := newLedgerAccount("019ff448-c43d-70d3-83c7-dfa0674469b7", account.Asset)
				restrictiveAccount := newLedgerAccount("019ff448-c43d-70d3-83c7-dfa0674469b8", account.Asset)
				restrictiveAccount.BalancePolicy = account.BalanceNonNegative
				repository := &transactionRepositoryFake{states: map[string]account.LedgerState{
					otherAccount.ID.String():       {AccountID: otherAccount.ID.String()},
					restrictiveAccount.ID.String(): {AccountID: restrictiveAccount.ID.String()},
				}}
				entityTransaction := newMultiEntryTransaction(otherAccount.ID.String(), restrictiveAccount.ID.String())

				// Act (When).
				err := service.AssignLedgerEntries(ctx, repository, entityTransaction, restrictiveAccount, otherAccount)

				// Assert (Then).
				Expect(errors.Is(err, fault.ErrInsufficientBalance)).To(BeTrue())
				Expect(entityTransaction.Entries[0].SequenceNumber).To(Equal(int64(0)))
			})
		})
	})
})

type transactionRepositoryFake struct {
	states    map[string]account.LedgerState
	findOrder []string
}

func (f *transactionRepositoryFake) Save(context.Context, *transaction.Transaction) error {
	return nil
}

func (f *transactionRepositoryFake) Find(context.Context, transaction.TransactionCriteria) (*transaction.Transaction, error) {
	return nil, transaction.ErrTransactionNotFound
}

func (f *transactionRepositoryFake) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	return false, nil
}

func (f *transactionRepositoryFake) FindLatestLedgerState(_ context.Context, accountID string) (int64, int64, error) {
	f.findOrder = append(f.findOrder, accountID)
	state := f.states[accountID]
	return state.SequenceNumber, state.RunningBalance, nil
}

func newLedgerAccount(id string, accountType account.Type) *account.Account {
	accountID, err := entity.NewID(id)
	Expect(err).NotTo(HaveOccurred())
	return &account.Account{
		ID:            accountID,
		AccountType:   accountType,
		BalancePolicy: account.BalanceUnrestricted,
		Currency:      money.BRL,
	}
}

func newMultiEntryTransaction(accountA, accountB string) *transaction.Transaction {
	transactionID := "019ff448-c43d-70d3-83c7-dfa0674469b9"
	entries := make([]*transaction.Entry, 0, 4)
	for _, data := range []struct {
		accountID string
		direction transaction.Direction
		amount    int64
	}{
		{accountA, transaction.Debit, 1000},
		{accountB, transaction.Credit, 600},
		{accountA, transaction.Debit, 500},
		{accountB, transaction.Credit, 900},
	} {
		amount, err := money.NewMoney(data.amount, money.BRL)
		Expect(err).NotTo(HaveOccurred())
		entry, err := transaction.NewEntryBuilder().WithID().WithAccountExternalID(uuid.NewString()).
			WithTransactionID(transactionID).WithDirection(data.direction).WithAmount(amount).Build()
		Expect(err).NotTo(HaveOccurred())
		entry.AssignAccountID(data.accountID)
		entries = append(entries, entry)
	}
	amount, err := money.NewMoney(1500, money.BRL)
	Expect(err).NotTo(HaveOccurred())
	entityTransaction, err := transaction.NewTransactionBuilder().WithID(transactionID).
		WithIdempotencyKey("ledger-entry-assigner").WithFingerprint("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd").
		WithStatus(transaction.Pending).WithAmount(amount).WithOperation(transaction.OperationTransfer).
		WithEntries(entries).Build()
	Expect(err).NotTo(HaveOccurred())
	return entityTransaction
}

func entryPositions(entries []*transaction.Entry) []int64 {
	values := make([]int64, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.TransactionPosition)
	}
	return values
}

func entrySequences(entries []*transaction.Entry) []int64 {
	values := make([]int64, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.SequenceNumber)
	}
	return values
}

func entryBalances(entries []*transaction.Entry) []int64 {
	values := make([]int64, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.RunningBalance)
	}
	return values
}
