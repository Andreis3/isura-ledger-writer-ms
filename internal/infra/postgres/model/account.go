package model

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
)

type Account struct {
	ID                pgtype.Text
	AccountExternalID pgtype.Text
	AccountNumber     pgtype.Text
	TaxID             pgtype.Text
	Status            pgtype.Text
	Type              pgtype.Text
	BalancePolicy     pgtype.Text
	Currency          pgtype.Text
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
}

func ToAccountModel(entity *account.Account) Account {
	return Account{
		ID: pgtype.Text{
			String: entity.ID.String(),
			Valid:  true,
		},
		AccountExternalID: pgtype.Text{
			String: entity.AccountExternalID,
			Valid:  true,
		},
		AccountNumber: pgtype.Text{
			String: entity.AccountNumber,
			Valid:  true,
		},
		TaxID: pgtype.Text{
			String: entity.TaxID,
			Valid:  true,
		},
		Status: pgtype.Text{
			String: string(entity.Status),
			Valid:  true,
		},
		Type: pgtype.Text{
			String: string(entity.AccountType),
			Valid:  true,
		},
		BalancePolicy: pgtype.Text{
			String: string(entity.BalancePolicy),
			Valid:  true,
		},
		Currency: pgtype.Text{
			String: string(entity.Currency),
			Valid:  true,
		},
		CreatedAt: pgtype.Timestamptz{
			Time:  entity.CreatedAt,
			Valid: true,
		},
		UpdatedAt: pgtype.Timestamptz{
			Time:  entity.UpdatedAt,
			Valid: true,
		},
	}
}

func ToAccountDomain(model Account) (*account.Account, error) {

	return account.NewAccountBuilder().
		WithID(model.ID.String).
		WithAccountExternalID(model.AccountExternalID.String).
		WithAccountNumber(model.AccountNumber.String).
		WithTaxID(model.TaxID.String).
		WithStatus(model.Status.String).
		WithType(model.Type.String).
		WithBalancePolicy(model.BalancePolicy.String).
		WithCurrency(model.Currency.String).
		WithCreatedAt(model.CreatedAt.Time).
		WithUpdatedAt(model.UpdatedAt.Time).
		Build()
}
