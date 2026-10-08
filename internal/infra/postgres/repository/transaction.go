package repository

import (
	"context"
	"errors"
	"fmt"

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

// FindLatestLedgerState returns the latest persisted sequence and balance for an account.
func (r *TransactionRepository) FindLatestLedgerState(ctx context.Context, accountID string) (int64, int64, error) {
	var sequenceNumber, runningBalance int64
	db := resolveDB(ctx, r.db)
	err := db.QueryRow(ctx, `
		SELECT sequence_number, running_balance
		FROM entries
		WHERE account_id = $1
		ORDER BY sequence_number DESC
		LIMIT 1`, accountID).Scan(&sequenceNumber, &runningBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("find latest ledger state: %w", err)
	}
	return sequenceNumber, runningBalance, nil
}

// Save persists a prepared transaction and its entries using the transaction carried by ctx.
func (r *TransactionRepository) Save(ctx context.Context, data *transaction.Transaction) error {
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
			(id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			entryModel.ID,
			entryModel.AccountID,
			entryModel.TransactionID,
			entryModel.SequenceNumber,
			entryModel.TransactionPosition,
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
		SELECT id, account_id, transaction_id, sequence_number, transaction_position, direction, amount,
			running_balance, currency, metadata, created_at
		FROM entries
		WHERE transaction_id = $1
		ORDER BY transaction_position`, transactionID)
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
			&entryModel.TransactionPosition,
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
