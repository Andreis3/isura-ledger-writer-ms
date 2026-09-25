package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
)

var ErrBackfillTransactionRequired = errors.New("historical balance backfill requires a transaction")

// BackfillReport describes the result of recalculating historical balances.
type BackfillReport struct {
	Accounts                   int
	Entries                    int
	UpdatedEntries             int
	PersistedBalanceMismatches int
	SequenceMismatches         int
	PolicyViolations           []AccountBalanceViolation
	ReadyForActivation         bool
}

// AccountBalanceViolation identifies an account whose recalculated normal
// balance does not satisfy its explicitly configured restrictive policy.
type AccountBalanceViolation struct {
	AccountID      string
	BalancePolicy  account.BalancePolicy
	RunningBalance int64
}

type historicalAccount struct {
	account    account.Account
	state      account.LedgerState
	hasEntries bool
}

// HistoricalBalanceBackfill recalculates running_balance without changing
// entry identity or sequence_number. Run it inside a Unit of Work transaction.
type HistoricalBalanceBackfill struct {
	db database.Querier
}

func NewHistoricalBalanceBackfill(db database.Querier) *HistoricalBalanceBackfill {
	return &HistoricalBalanceBackfill{db: db}
}

// Run recalculates all account entries in deterministic sequence order. It
// updates only running_balance and commits only when the caller commits the
// transaction carried by ctx.
func (b *HistoricalBalanceBackfill) Run(ctx context.Context) (BackfillReport, error) {
	if _, ok := database.ExtractTx(ctx); !ok {
		return BackfillReport{}, ErrBackfillTransactionRequired
	}

	report := BackfillReport{ReadyForActivation: true}
	accounts, err := b.loadAccounts(ctx)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("load accounts for historical balance backfill: %w", err)
	}
	report.Accounts = len(accounts)

	if err := b.recalculateEntries(ctx, accounts, &report); err != nil {
		return BackfillReport{}, err
	}
	for _, historical := range accounts {
		if !historical.hasEntries {
			continue
		}
		if historical.account.BalancePolicy == account.BalanceNonNegative && historical.state.RunningBalance < 0 {
			report.PolicyViolations = append(report.PolicyViolations, AccountBalanceViolation{
				AccountID:      historical.account.ID.String(),
				BalancePolicy:  historical.account.BalancePolicy,
				RunningBalance: historical.state.RunningBalance,
			})
		}
	}
	// Balance mismatches are resolved by the idempotent updates above. Sequence
	// mismatches and restrictive-policy violations require operator action.
	report.ReadyForActivation = report.SequenceMismatches == 0 && len(report.PolicyViolations) == 0
	return report, nil
}

func (b *HistoricalBalanceBackfill) loadAccounts(ctx context.Context) ([]*historicalAccount, error) {
	db := resolveDB(ctx, b.db)
	rows, err := db.Query(ctx, `
		SELECT id, type, balance_policy, currency
		FROM accounts
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]*historicalAccount, 0)
	for rows.Next() {
		var id, accountingType, policy, currency pgtype.Text
		if err := rows.Scan(&id, &accountingType, &policy, &currency); err != nil {
			return nil, err
		}
		if !id.Valid || !policy.Valid || !account.BalancePolicy(policy.String).IsValid() {
			return nil, fmt.Errorf("account %q has invalid or missing balance policy", id.String)
		}
		accountID, err := entity.NewID(id.String)
		if err != nil {
			return nil, fmt.Errorf("account %q has invalid id: %w", id.String, err)
		}
		accounts = append(accounts, &historicalAccount{
			account: account.Account{
				ID:            accountID,
				AccountType:   account.Type(accountingType.String),
				BalancePolicy: account.BalancePolicy(policy.String),
				Currency:      money.Currency(currency.String),
			},
			state: account.LedgerState{AccountID: id.String},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (b *HistoricalBalanceBackfill) recalculateEntries(ctx context.Context, accounts []*historicalAccount, report *BackfillReport) error {
	byID := make(map[string]*historicalAccount, len(accounts))
	for _, historical := range accounts {
		byID[historical.account.ID.String()] = historical
	}
	db := resolveDB(ctx, b.db)
	rows, err := db.Query(ctx, `
		SELECT account_id, sequence_number, direction, amount, currency, running_balance
		FROM entries
		ORDER BY account_id, sequence_number`)
	if err != nil {
		return fmt.Errorf("load historical entries: %w", err)
	}
	defer rows.Close()

	type historicalEntry struct {
		accountID        string
		sequence         int64
		direction        transaction.Direction
		amount           int64
		currency         money.Currency
		persistedBalance int64
	}
	entries := make([]historicalEntry, 0)
	for rows.Next() {
		var accountID, direction, currency pgtype.Text
		var sequence, amount, persistedBalance pgtype.Int8
		if err := rows.Scan(&accountID, &sequence, &direction, &amount, &currency, &persistedBalance); err != nil {
			return fmt.Errorf("scan historical entry: %w", err)
		}
		_, ok := byID[accountID.String]
		if !ok {
			return fmt.Errorf("entry %q references unknown account", accountID.String)
		}
		entries = append(entries, historicalEntry{
			accountID:        accountID.String,
			sequence:         sequence.Int64,
			direction:        transaction.Direction(direction.String),
			amount:           amount.Int64,
			currency:         money.Currency(currency.String),
			persistedBalance: persistedBalance.Int64,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate historical entries: %w", err)
	}
	rows.Close()

	for _, entry := range entries {
		historical := byID[entry.accountID]
		entryAmount, err := money.NewMoney(entry.amount, entry.currency)
		if err != nil {
			return fmt.Errorf("build historical entry amount for account %q: %w", entry.accountID, err)
		}
		newState, err := historical.account.ApplyHistoricalEntry(historical.state, entry.direction, entryAmount)
		if err != nil {
			return fmt.Errorf("recalculate historical entry for account %q: %w", entry.accountID, err)
		}
		report.Entries++
		historical.hasEntries = true
		if newState.SequenceNumber != entry.sequence {
			report.SequenceMismatches++
		}
		if newState.RunningBalance != entry.persistedBalance {
			report.PersistedBalanceMismatches++
		}
		if newState.RunningBalance != entry.persistedBalance {
			if _, err := db.Exec(ctx, `UPDATE entries SET running_balance = $1 WHERE account_id = $2 AND sequence_number = $3`, newState.RunningBalance, entry.accountID, entry.sequence); err != nil {
				return fmt.Errorf("update historical balance for account %q sequence %d: %w", entry.accountID, entry.sequence, err)
			}
			report.UpdatedEntries++
		}
		historical.state = newState
		historical.state.SequenceNumber = entry.sequence
	}
	return nil
}
