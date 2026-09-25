package repository

import (
	"context"
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/jackc/pgx/v5"
)

type AccountRepository struct {
	db database.Querier
}

func NewAccountRepository(db database.Querier) *AccountRepository {
	return &AccountRepository{
		db: db,
	}
}

func (r *AccountRepository) Save(ctx context.Context, account *account.Account) error {
	db := resolveDB(ctx, r.db)

	query := `INSERT INTO 
    accounts (id, 
              account_external_id,
              account_number,
              tax_id,
              status,
              type,
              balance_policy,
              currency,
              created_at,
              updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	accountModel := model.ToAccountModel(account)

	_, err := db.Exec(ctx, query,
		accountModel.ID,
		accountModel.AccountExternalID,
		accountModel.AccountNumber,
		accountModel.TaxID,
		accountModel.Status,
		accountModel.Type,
		accountModel.BalancePolicy,
		accountModel.Currency,
		accountModel.CreatedAt,
		accountModel.UpdatedAt,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return fault.SaveAccountAlreadyExistsError(err)
		}
		return fault.SaveAccountError(err)
	}

	return nil
}

func (r *AccountRepository) FindAccount(ctx context.Context, params criteria.AccountCriteria) (*account.Account, error) {
	db := resolveDB(ctx, r.db)

	baseQuery := `
        SELECT 
            id, 
            account_external_id, 
            account_number, 
            tax_id,
            status,
            type,
            balance_policy,
            currency, 
            created_at, 
            updated_at 
        FROM accounts 
        WHERE 1=1
    `
	query, args := criteria.GetAccountCriteria(baseQuery, params)
	var accountModel model.Account
	err := db.QueryRow(ctx, query, args...).Scan(
		&accountModel.ID,
		&accountModel.AccountExternalID,
		&accountModel.AccountNumber,
		&accountModel.TaxID,
		&accountModel.Status,
		&accountModel.Type,
		&accountModel.BalancePolicy,
		&accountModel.Currency,
		&accountModel.CreatedAt,
		&accountModel.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Retorna erro padronizado de não encontrado
		}
		return nil, fault.FindAccountError(err)
	}

	accDomain, err := model.ToAccountDomain(accountModel)
	if err != nil {
		return nil, err
	}

	return accDomain, nil
}
