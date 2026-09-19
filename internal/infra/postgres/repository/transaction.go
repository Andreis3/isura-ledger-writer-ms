package repository

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

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
// Account rows are locked before sequences are calculated, so concurrent writers for an
// account cannot derive the same next sequence.
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
	type accountLedgerState struct {
		nextSequence   int64
		runningBalance int64
	}

	ordered := append([]*transaction.Entry(nil), entries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].AccountID < ordered[j].AccountID
	})

	db := resolveDB(ctx, r.db)
	stateByAccount := make(map[string]accountLedgerState, len(ordered))
	for _, entry := range ordered {
		if state, ok := stateByAccount[entry.AccountID]; ok {
			state.nextSequence++
			state.runningBalance = applyEntryToBalance(state.runningBalance, entry)
			stateByAccount[entry.AccountID] = state
			entry.SetSequenceNumber(state.nextSequence)
			entry.SetRunningBalance(state.runningBalance)
			continue
		}

		var accountID string
		if err := db.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1 FOR UPDATE`, entry.AccountID).Scan(&accountID); err != nil {
			return err
		}
		var lastSequence, lastBalance int64
		if err := db.QueryRow(ctx, `
			SELECT sequence_number, running_balance
			FROM entries
			WHERE account_id = $1
			ORDER BY sequence_number DESC
			LIMIT 1`, entry.AccountID).Scan(&lastSequence, &lastBalance); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		next := lastSequence + 1
		balance := applyEntryToBalance(lastBalance, entry)
		stateByAccount[entry.AccountID] = accountLedgerState{nextSequence: next, runningBalance: balance}
		entry.SetSequenceNumber(next)
		entry.SetRunningBalance(balance)
	}
	return nil
}

func applyEntryToBalance(balance int64, entry *transaction.Entry) int64 {
	if entry.Direction == transaction.Debit {
		return balance - entry.Amount.Amount()
	}
	return balance + entry.Amount.Amount()
}

func (r *TransactionRepository) Find(ctx context.Context, params criteria.TransactionCriteria) (*transaction.Transaction, error) {
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
