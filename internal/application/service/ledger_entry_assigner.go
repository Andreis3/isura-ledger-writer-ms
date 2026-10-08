package service

import (
	"context"
	"sort"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

// AssignLedgerEntries applies a transaction's entries in input order and assigns
// the resulting position, sequence, and running balance before persistence.
func AssignLedgerEntries(
	ctx context.Context,
	transactionsRepository transaction.Repository,
	entityTransaction *transaction.Transaction,
	accounts ...*account.Account,
) error {
	accountsByID := make(map[string]*account.Account, len(accounts))
	accountIDs := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		id := acc.ID.String()
		if _, exists := accountsByID[id]; exists {
			continue
		}
		accountsByID[id] = acc
		accountIDs = append(accountIDs, id)
	}
	sort.Strings(accountIDs)

	statesByAccount := make(map[string]account.LedgerState, len(accountIDs))
	for _, accountID := range accountIDs {
		sequenceNumber, runningBalance, err := transactionsRepository.FindLatestLedgerState(ctx, accountID)
		if err != nil {
			return err
		}
		statesByAccount[accountID] = account.LedgerState{
			AccountID: accountID, SequenceNumber: sequenceNumber, RunningBalance: runningBalance,
		}
	}

	assignments := make([]entryAssignment, 0, len(entityTransaction.Entries))
	for position, entry := range entityTransaction.Entries {
		acc, exists := accountsByID[entry.AccountID]
		if !exists {
			return fault.FindAccountNotFoundError(account.ErrAccountNotFound)
		}
		state, exists := statesByAccount[entry.AccountID]
		if !exists {
			return fault.FindAccountNotFoundError(account.ErrAccountNotFound)
		}
		updatedState, err := acc.ApplyEntry(state, entry.Direction, entry.Amount)
		if err != nil {
			return err
		}

		statesByAccount[entry.AccountID] = updatedState
		assignments = append(assignments, entryAssignment{entry: entry, position: int64(position), state: updatedState})
	}
	for _, assignment := range assignments {
		assignment.entry.SetTransactionPosition(assignment.position)
		assignment.entry.SetSequenceNumber(assignment.state.SequenceNumber)
		assignment.entry.SetRunningBalance(assignment.state.RunningBalance)
	}
	return nil
}

type entryAssignment struct {
	entry    *transaction.Entry
	position int64
	state    account.LedgerState
}
