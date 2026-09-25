package account

import (
	"errors"
	"math"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
)

// BalancePolicy defines the rule applied to an account's normal balance.
type BalancePolicy string

const (
	BalanceNonNegative  BalancePolicy = "BALANCE_NON_NEGATIVE"
	BalanceUnrestricted BalancePolicy = "BALANCE_UNRESTRICTED"
)

// IsValid reports whether the policy is supported by the ledger.
func (p BalancePolicy) IsValid() bool {
	return p == BalanceNonNegative || p == BalanceUnrestricted
}

// LedgerState is the last confirmed state of an account ledger stream.
type LedgerState struct {
	AccountID      string
	SequenceNumber int64
	RunningBalance int64
}

// IsValid reports whether the state can be used as an initial ledger state.
func (s LedgerState) IsValid() bool {
	return s.SequenceNumber >= 0
}

var (
	ErrInvalidLedgerState    = errors.New("invalid ledger state")
	ErrLedgerBalanceOverflow = errors.New("ledger balance overflow")
)

// ApplyEntry applies one entry without mutating the supplied state.
func (a Account) ApplyEntry(state LedgerState, direction transaction.Direction, amount money.Money) (LedgerState, error) {
	if !a.BalancePolicy.IsValid() {
		return state, fault.ErrInvalidBalancePolicy
	}
	if !state.IsValid() {
		return state, ErrInvalidLedgerState
	}
	if !direction.IsValid() {
		return state, transaction.ErrInvalidDirection
	}
	if amount.IsZero() {
		return state, transaction.ErrAmountEqualZero
	}
	if amount.IsNegative() {
		return state, transaction.ErrNegativeAmountValue
	}
	if !amount.Currency().IsValid() {
		return state, money.ErrInvalidCurrency
	}
	if !a.Currency.IsValid() {
		return state, money.ErrInvalidCurrency
	}
	if a.Currency != amount.Currency() {
		return state, money.ErrCurrencyMismatch
	}

	delta, err := balanceDelta(a.AccountType, direction, amount.Amount())
	if err != nil {
		return state, err
	}
	newBalance, err := addBalance(state.RunningBalance, delta)
	if err != nil {
		return state, err
	}
	if a.BalancePolicy == BalanceNonNegative && newBalance < 0 {
		return state, fault.ErrInsufficientBalance
	}
	if state.SequenceNumber == math.MaxInt64 {
		return state, ErrLedgerBalanceOverflow
	}

	state.SequenceNumber++
	state.RunningBalance = newBalance
	return state, nil
}

func balanceDelta(accountType Type, direction transaction.Direction, amount int64) (int64, error) {
	debitIncreases := accountType == Asset || accountType == Expense
	creditIncreases := accountType == Liability || accountType == Revenue || accountType == Equity
	if !debitIncreases && !creditIncreases {
		return 0, ErrInvalidAccountingType
	}

	increases := (direction == transaction.Debit && debitIncreases) ||
		(direction == transaction.Credit && creditIncreases)
	if increases {
		return amount, nil
	}
	if amount == math.MinInt64 {
		return 0, ErrLedgerBalanceOverflow
	}
	return -amount, nil
}

func addBalance(balance, delta int64) (int64, error) {
	if delta > 0 && balance > math.MaxInt64-delta {
		return 0, ErrLedgerBalanceOverflow
	}
	if delta < 0 && balance < math.MinInt64-delta {
		return 0, ErrLedgerBalanceOverflow
	}
	return balance + delta, nil
}
