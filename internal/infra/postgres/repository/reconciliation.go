package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
)

var ErrReconciliationTransactionRequired = errors.New("ledger reconciliation requires a transaction")

// ReconciliationReport summarizes the result of replaying every account ledger.
type ReconciliationReport struct {
	AccountsChecked int                      `json:"accounts_checked"`
	EntriesChecked  int                      `json:"entries_checked"`
	MismatchCount   int                      `json:"mismatch_count"`
	Reconciled      bool                     `json:"reconciled"`
	Mismatches      []ReconciliationMismatch `json:"mismatches"`
}

// ReconciliationMismatch describes the latest persisted balance that differs from replay.
type ReconciliationMismatch struct {
	AccountID        string `json:"account_id"`
	EntriesChecked   int    `json:"entries_checked"`
	ExpectedBalance  int64  `json:"expected_balance"`
	PersistedBalance int64  `json:"persisted_balance"`
	Currency         string `json:"currency"`
}

type reconciliationAccount struct {
	account account.Account
	state   account.LedgerState
	count   int
	latest  int64
}

type LedgerReconciliation struct {
	db database.Querier
}

func NewLedgerReconciliation(db database.Querier) *LedgerReconciliation {
	return &LedgerReconciliation{db: db}
}

// Run replays account entries using the caller's active transaction.
func (r *LedgerReconciliation) Run(ctx context.Context) (ReconciliationReport, error) {
	if _, ok := database.ExtractTx(ctx); !ok {
		return ReconciliationReport{}, ErrReconciliationTransactionRequired
	}

	accounts, err := r.loadAccounts(ctx)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("load accounts for reconciliation: %w", err)
	}
	return r.replayEntries(ctx, accounts)
}

func (r *LedgerReconciliation) loadAccounts(ctx context.Context) (map[string]*reconciliationAccount, error) {
	rows, err := resolveDB(ctx, r.db).Query(ctx, `
		SELECT id, type, balance_policy, currency
		FROM accounts
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	accounts := make(map[string]*reconciliationAccount)
	for rows.Next() {
		var id, accountingType, policy, currency pgtype.Text
		if err := rows.Scan(&id, &accountingType, &policy, &currency); err != nil {
			return nil, fmt.Errorf("scan account for reconciliation: %w", err)
		}
		if !id.Valid || !accountingType.Valid || !policy.Valid || !currency.Valid {
			return nil, errors.New("account has missing reconciliation fields")
		}
		accountID, err := entity.NewID(id.String)
		if err != nil {
			return nil, fmt.Errorf("account %q has invalid id: %w", id.String, err)
		}
		if !account.Type(accountingType.String).IsValid() {
			return nil, fmt.Errorf("account %q has invalid accounting type %q", id.String, accountingType.String)
		}
		accountEntity := account.Account{
			ID: accountID, AccountType: account.Type(accountingType.String),
			BalancePolicy: account.BalancePolicy(policy.String), Currency: money.Currency(currency.String),
		}
		if !accountEntity.BalancePolicy.IsValid() {
			return nil, fmt.Errorf("account %q has invalid balance policy %q", id.String, policy.String)
		}
		if !accountEntity.Currency.IsValid() {
			return nil, fmt.Errorf("account %q has invalid currency %q", id.String, currency.String)
		}
		accounts[id.String] = &reconciliationAccount{
			account: accountEntity,
			state:   account.LedgerState{AccountID: id.String},
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts for reconciliation: %w", err)
	}
	return accounts, nil
}

func (r *LedgerReconciliation) replayEntries(ctx context.Context, accounts map[string]*reconciliationAccount) (ReconciliationReport, error) {
	rows, err := resolveDB(ctx, r.db).Query(ctx, `
		SELECT account_id, sequence_number, direction, amount, currency, running_balance
		FROM entries
		ORDER BY account_id, sequence_number`)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("query entries for reconciliation: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountID, direction, currency pgtype.Text
		var sequence, amount, persisted pgtype.Int8
		if err := rows.Scan(&accountID, &sequence, &direction, &amount, &currency, &persisted); err != nil {
			return ReconciliationReport{}, fmt.Errorf("scan entry for reconciliation: %w", err)
		}
		if !accountID.Valid || !sequence.Valid || !direction.Valid || !amount.Valid || !currency.Valid || !persisted.Valid {
			return ReconciliationReport{}, errors.New("entry has missing reconciliation fields")
		}
		if sequence.Int64 <= 0 {
			return ReconciliationReport{}, fmt.Errorf("entry for account %q has invalid sequence %d", accountID.String, sequence.Int64)
		}
		historical, ok := accounts[accountID.String]
		if !ok {
			return ReconciliationReport{}, fmt.Errorf("entry references unknown account %q", accountID.String)
		}
		if err := applyReconciliationEntry(historical, sequence.Int64, transaction.Direction(direction.String), amount.Int64, money.Currency(currency.String)); err != nil {
			return ReconciliationReport{}, fmt.Errorf("replay entry for account %q sequence %d: %w", accountID.String, sequence.Int64, err)
		}
		historical.latest = persisted.Int64
	}
	if err := rows.Err(); err != nil {
		return ReconciliationReport{}, fmt.Errorf("iterate entries for reconciliation: %w", err)
	}
	return buildReconciliationReport(accounts), nil
}

func applyReconciliationEntry(historical *reconciliationAccount, sequence int64, direction transaction.Direction, amount int64, currency money.Currency) error {
	entryAmount, err := money.NewMoney(amount, currency)
	if err != nil {
		return fmt.Errorf("build amount: %w", err)
	}
	state, err := historical.account.ApplyHistoricalEntry(historical.state, direction, entryAmount)
	if err != nil {
		return err
	}
	historical.state = state
	historical.state.SequenceNumber = sequence
	historical.count++
	return nil
}

func buildReconciliationReport(accounts map[string]*reconciliationAccount) ReconciliationReport {
	report := ReconciliationReport{
		AccountsChecked: len(accounts),
		Reconciled:      true,
		Mismatches:      make([]ReconciliationMismatch, 0),
	}
	accountIDs := make([]string, 0, len(accounts))
	for id := range accounts {
		accountIDs = append(accountIDs, id)
	}
	sort.Strings(accountIDs)
	for _, id := range accountIDs {
		historical := accounts[id]
		report.EntriesChecked += historical.count
		if historical.count == 0 || historical.state.RunningBalance == historical.latest {
			continue
		}
		report.Mismatches = append(report.Mismatches, ReconciliationMismatch{
			AccountID: id, EntriesChecked: historical.count,
			ExpectedBalance:  historical.state.RunningBalance,
			PersistedBalance: historical.latest,
			Currency:         string(historical.account.Currency),
		})
	}
	report.MismatchCount = len(report.Mismatches)
	report.Reconciled = report.MismatchCount == 0
	return report
}
