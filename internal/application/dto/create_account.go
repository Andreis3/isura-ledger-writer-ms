package dto

import (
	"log/slog"

	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
)

type CreateAccountInput struct {
	AccountExternalID string `json:"account_external_id"`
	AccountNumber     string `json:"account_number"`
	TaxID             string `json:"tax_id"`
	AccountType       string `json:"account_type"`
	BalancePolicy     string `json:"balance_policy"`
	Currency          string `json:"currency"`
}

type CreateAccountOutput struct {
	AccountID *string `json:"account_id"`
}

func (d *CreateAccountInput) CreateAccountFacade() (*account.Account, error) {
	return account.NewAccountBuilder().
		WithID().
		WithAccountExternalID(d.AccountExternalID).
		WithAccountNumber(d.AccountNumber).
		WithTaxID(d.TaxID).
		WithStatus().
		WithType(d.AccountType).
		WithBalancePolicy(d.BalancePolicy).
		WithCurrency(d.Currency).
		WithCreatedAt().
		WithUpdatedAt().
		Build()
}

// LogValue implements the slog.LogValuer interface statically and without reflection.
func (d CreateAccountInput) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("account_external_id", d.AccountExternalID),
		slog.String("tax_id", MaskMiddleVisible(d.TaxID)),
		slog.String("account_number", MaskTotal(d.AccountNumber)),
		slog.String("account_type", d.AccountType),
		slog.String("balance_policy", d.BalancePolicy),
		slog.String("currency", d.Currency),
	)
}
