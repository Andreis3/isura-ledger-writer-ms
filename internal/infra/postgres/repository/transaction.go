package repository

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/jackc/pgx/v5"

	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
)

type TransactionRepository struct {
	db database.Querier
}

func NewTransactionRepository(db database.Querier) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Save(ctx context.Context, data *transaction.Transaction) error {
	batch := pgx.Batch{}

	transactionModel := model.ToTransactionModel(data)

	batch.Queue(`
		INSERT INTO transactions 
			(id, idempotency_key, status, operation, amount, currency, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		transactionModel.ID,
		transactionModel.IdempotencyKey,
		transactionModel.Status,
		transactionModel.Operation,
		transactionModel.Amount,
		transactionModel.Currency,
		transactionModel.CreatedAt,
		transactionModel.UpdatedAt)

	for _, entry := range data.Entries {
		entryModel := model.ToEntryModel(entry)
		batch.Queue(`
			INSERT INTO entries 
				(id, account_id, transaction_id, direction, amount, currency, created_at) 
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			entryModel.ID,
			entryModel.AccountID,
			entryModel.TransactionID,
			entryModel.Direction,
			entryModel.Amount,
			entryModel.Currency,
			entryModel.CreatedAt)
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

func (r *TransactionRepository) Find(ctx context.Context, params criteria.TransactionCriteria) (*transaction.Transaction, error) {
	db := resolveDB(ctx, r.db)

	batch := pgx.Batch{}

	baseQueryTransaction := `
		SELECT 
		    id, 
		    idempotency_key, 
		    status,
		    amount,
		    operation,
		    currency,
		    created_at, 
		    updated_at	
		FROM transactions	
		WHERE 1 = 1
		`

	queryTransaction, argsTransaction := criteria.GetTransactionCriteria(baseQueryTransaction, params)
	batch.Queue(queryTransaction, argsTransaction...)

	if params.WithEntries {
		baseQueryEntries := `
			SELECT 
				id, 
				idempotency_key, 
				direction, 
				amount, 
				currency, 
				account_id, 
				transaction_id, 
				created_at
			FROM entries
			WHERE 1 = 1
			`

		queryEntries, argsEntries := criteria.GetEntryCriteria(baseQueryEntries, criteria.EntryCriteria{
			TransactionID: params.ID,
		})

		batch.Queue(queryEntries, argsEntries)

	}

	results := db.SendBatch(ctx, &batch)
	defer results.Close()

	// first result transaction
	transactionRow := results.QueryRow()
	var transactionModel model.Transaction
	if err := transactionRow.Scan(
		&transactionModel.ID,
		&transactionModel.IdempotencyKey,
		&transactionModel.Status,
		&transactionModel.Amount,
		&transactionModel.Operation,
		&transactionModel.Currency,
		&transactionModel.CreatedAt,
		&transactionModel.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, transaction.ErrTransactionNotFound
		}
		return nil, err
	}

	// second result entries
	//entryRows, err := results.Query()
	//if err != nil {
	//	return nil, err
	//}
	//defer entryRows.Close()

	var entries []*transaction.Entry
	//for entryRows.Next() {
	//	var entryModel model.Entry
	//	if err := entryRows.Scan(
	//		&entryModel.ID,
	//		&entryModel.Direction,
	//		&entryModel.Amount,
	//		&entryModel.Currency,
	//		&entryModel.AccountID,
	//		&entryModel.TransactionID,
	//		&entryModel.CreatedAt,
	//	); err != nil {
	//		return nil, err
	//	}
	//
	//	entry, err := model.ToEntryDomain(entryModel)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	entries = append(entries, entry)
	//}
	//
	//if err := entryRows.Err(); err != nil {
	//	return nil, err
	//}

	return model.ToTransactionDomain(transactionModel, entries)

}

func (r *TransactionRepository) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	db := resolveDB(ctx, r.db)
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM transactions WHERE idempotency_key = $1)
	`, idempotencyKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
