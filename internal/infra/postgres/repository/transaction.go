package repository

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

type TransactionRepository struct {
	db database.Querier
}

func NewTransactionRepository(db database.Querier) *TransactionRepository {
	return &TransactionRepository{db: db}
}

// Save persists the transaction and its entries using the transaction carried by ctx.
func (r *TransactionRepository) Save(ctx context.Context, data *transaction.Transaction) error {
	if err := r.assignSequences(ctx, data.Entries); err != nil {
		return err
	}

	transactionModel, err := model.ToTransactionModel(data)
	if err != nil {
		return err
	}
	batch := pgx.Batch{}
	batch.Queue(`
		INSERT INTO transactions
			(id, idempotency_key, request_fingerprint, status, operation, amount, currency, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		transactionModel.ID,
		transactionModel.IdempotencyKey,
		transactionModel.RequestFingerprint,
		transactionModel.Status,
		transactionModel.Operation,
		transactionModel.Amount,
		transactionModel.Currency,
		transactionModel.Metadata,
		transactionModel.CreatedAt,
		transactionModel.UpdatedAt,
	)

	for _, entry := range data.Entries {
		entryModel, err := model.ToEntryModel(entry)
		if err != nil {
			return err
		}
		batch.Queue(`
			INSERT INTO entries
				(id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			entryModel.ID,
			entryModel.AccountID,
			entryModel.TransactionID,
			entryModel.SequenceNumber,
			entryModel.Direction,
			entryModel.Amount,
			entryModel.RunningBalance,
			entryModel.Currency,
			entryModel.Metadata,
			entryModel.CreatedAt,
		)
	}

	db := resolveDB(ctx, r.db)
	results := db.SendBatch(ctx, &batch)
	defer results.Close()
	if _, err := results.Exec(); err != nil {
		return err
	}
	for range data.Entries {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (r *TransactionRepository) assignSequences(ctx context.Context, entries []*transaction.Entry) error {
	ordered := append([]*transaction.Entry(nil), entries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].AccountID < ordered[j].AccountID
	})

	db := resolveDB(ctx, r.db)
	stateByAccount := make(map[string]account.LedgerState, len(ordered))
	accountsByID := make(map[string]account.Account, len(ordered))
	assignments := make([]entryAssignment, 0, len(ordered))
	for _, entry := range ordered {
		state, ok := stateByAccount[entry.AccountID]
		if !ok {
			loadedAccount, err := r.readAccountLedgerConfig(ctx, db, entry.AccountID)
			if err != nil {
				return err
			}
			accountsByID[entry.AccountID] = loadedAccount

			state, err = r.readEntryLedgerState(ctx, db, entry.AccountID)
			if err != nil {
				return err
			}
		}

		updatedState, err := accountsByID[entry.AccountID].ApplyEntry(
			state,
			entry.Direction,
			entry.Amount,
		)
		if err != nil {
			return err
		}
		stateByAccount[entry.AccountID] = updatedState
		assignments = append(assignments, entryAssignment{
			entry: entry,
			state: updatedState,
		})
	}

	for _, assignment := range assignments {
		assignment.entry.SetSequenceNumber(assignment.state.SequenceNumber)
		assignment.entry.SetRunningBalance(assignment.state.RunningBalance)
	}
	return nil
}

type entryAssignment struct {
	entry *transaction.Entry
	state account.LedgerState
}

func (r *TransactionRepository) readAccountLedgerConfig(ctx context.Context, db database.Querier, accountID string) (account.Account, error) {
	var accountType, balancePolicy, currency string
	err := db.QueryRow(ctx, `
		SELECT type, balance_policy, currency
		FROM accounts
		WHERE id = $1`, accountID).Scan(&accountType, &balancePolicy, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.Account{}, account.ErrAccountNotFound
	}
	if err != nil {
		return account.Account{}, err
	}
	return account.Account{
		AccountType:   account.Type(accountType),
		BalancePolicy: account.BalancePolicy(balancePolicy),
		Currency:      money.Currency(currency),
	}, nil
}

// readEntryLedgerState returns the last persisted state or the empty state for a new account.
func (r *TransactionRepository) readEntryLedgerState(ctx context.Context, db database.Querier, accountID string) (account.LedgerState, error) {
	state := account.LedgerState{AccountID: accountID}
	err := db.QueryRow(ctx, `
		SELECT sequence_number, running_balance
		FROM entries
		WHERE account_id = $1
		ORDER BY sequence_number DESC
		LIMIT 1`, accountID).Scan(&state.SequenceNumber, &state.RunningBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return account.LedgerState{}, err
	}
	return state, nil
}

func (r *TransactionRepository) Find(ctx context.Context, params transaction.TransactionCriteria) (*transaction.Transaction, error) {
	db := resolveDB(ctx, r.db)
	query, args := criteria.GetTransactionCriteria(`
		SELECT id, idempotency_key, request_fingerprint, status, amount, operation,
			currency, metadata, created_at, updated_at
		FROM transactions
		WHERE 1 = 1`, params)

	var transactionModel model.Transaction
	if err := db.QueryRow(ctx, query, args...).Scan(
		&transactionModel.ID,
		&transactionModel.IdempotencyKey,
		&transactionModel.RequestFingerprint,
		&transactionModel.Status,
		&transactionModel.Amount,
		&transactionModel.Operation,
		&transactionModel.Currency,
		&transactionModel.Metadata,
		&transactionModel.CreatedAt,
		&transactionModel.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, transaction.ErrTransactionNotFound
		}
		return nil, err
	}

	withEntries := params.WithEntries || transactionModel.Operation.String == string(transaction.OperationTransfer)
	entries, err := r.findEntries(ctx, transactionModel.ID.String, withEntries)
	if err != nil {
		return nil, err
	}
	return model.ToTransactionDomain(transactionModel, entries)
}

func (r *TransactionRepository) findEntries(ctx context.Context, transactionID string, withEntries bool) ([]*transaction.Entry, error) {
	if !withEntries {
		return nil, nil
	}
	db := resolveDB(ctx, r.db)
	rows, err := db.Query(ctx, `
		SELECT id, account_id, transaction_id, sequence_number, direction, amount,
			running_balance, currency, metadata, created_at
		FROM entries
		WHERE transaction_id = $1
		ORDER BY sequence_number`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*transaction.Entry, 0, 2)
	for rows.Next() {
		var entryModel model.Entry
		if err := rows.Scan(
			&entryModel.ID,
			&entryModel.AccountID,
			&entryModel.TransactionID,
			&entryModel.SequenceNumber,
			&entryModel.Direction,
			&entryModel.Amount,
			&entryModel.RunningBalance,
			&entryModel.Currency,
			&entryModel.Metadata,
			&entryModel.CreatedAt,
		); err != nil {
			return nil, err
		}
		entry, err := model.ToEntryDomain(entryModel)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *TransactionRepository) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	db := resolveDB(ctx, r.db)
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM transactions WHERE idempotency_key = $1)`, idempotencyKey).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
