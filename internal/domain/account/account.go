package account

import (
	"errors"
	"strings"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/shared"
	"github.com/andreis3/isura-ledger-ms/internal/domain/tax"
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

var (
	ErrInvalidAccountingType = errors.New("invalid accounting type")
	ErrEmptyExternalID       = errors.New("external id cannot be empty")
)

type AccountID string

type Type string

const (
	Asset     Type = "ASSET"     // Asset (e.g., Cash, Central Bank Accounts, Accounts Receivable)
	Liability Type = "LIABILITY" // Liability (e.g., Customer current account balances)
	Equity    Type = "EQUITY"    // Equity (e.g., Share capital)
	Revenue   Type = "REVENUE"   // Revenue (e.g., Pix and bank slip fees charged)
	Expense   Type = "EXPENSE"   // Expenses (e.g., Operational costs)
)

func (t Type) IsValid() bool {
	switch t {
	case Asset, Liability, Equity, Revenue, Expense:
		return true
	}
	return false
}

type Status string

const (
	StatusActive  Status = "ACTIVE"
	StatusBlocked Status = "BLOCKED"
	StatusClosed  Status = "CLOSED"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusBlocked, StatusClosed:
		return true
	}
	return false
}

// AccountBuilder builds an Account step by step with validation.
type AccountBuilder struct {
	id                entity.ID
	accountExternalID string
	accountNumber     string
	taxID             string
	status            Status
	accountType       Type
	balancePolicy     BalancePolicy
	currency          money.Currency
	createdAt         time.Time
	updatedAt         time.Time
	eval              validator.Evaluator
}

// NewAccountBuilder starts the build process.
func NewAccountBuilder() *AccountBuilder {
	return &AccountBuilder{}
}

type Account struct {
	ID                entity.ID
	AccountExternalID string
	AccountNumber     string
	TaxID             string
	Status            Status
	AccountType       Type
	BalancePolicy     BalancePolicy
	Currency          money.Currency
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// WithBalancePolicy sets the balance policy used by the account ledger.
func (b *AccountBuilder) WithBalancePolicy(policy string) *AccountBuilder {
	balancePolicy := BalancePolicy(policy)
	b.eval.CheckField(balancePolicy.IsValid(), "balance_policy", "invalid balance policy")
	b.balancePolicy = balancePolicy
	return b
}

// WithID sets the ID (required).
func (b *AccountBuilder) WithID(id ...string) *AccountBuilder {
	if len(id) > 0 {
		b.eval.CheckField(validator.NotBlank(id[0]), "id", "cannot be blank")
		b.eval.CheckField(validator.MatchesUUIDv7(id[0]), "id", "is not uuid v7")
		uuidV7, err := entity.NewID(id[0])
		if err != nil {
			b.eval.AddFieldError("id", err.Error())
		}

		b.id = uuidV7
		return b
	}

	uuidv7, err := entity.NewIDV7()
	if err != nil {
		b.eval.AddFieldError("id", err.Error())
	}

	b.id = uuidv7
	return b
}

// WithAccountExternalID sets the external ID (required).
func (b *AccountBuilder) WithAccountExternalID(externalID string) *AccountBuilder {
	b.eval.CheckField(validator.NotBlank(externalID), "account_external_id", "cannot be blank")
	b.eval.CheckField(validator.MatchesUUID(externalID), "account_external_id", "is not uuid")
	b.accountExternalID = externalID
	return b
}

// WithAccountNumber sets the account number (required).
func (b *AccountBuilder) WithAccountNumber(number string) *AccountBuilder {
	b.eval.CheckField(validator.NotBlank(number), "account_number", "cannot be blank")
	b.eval.CheckField(validator.MatchesNumber(number), "account_number", "is not a number")
	b.accountNumber = number
	return b
}

// WithTaxID defines CNPJ (required).
func (b *AccountBuilder) WithTaxID(rawTaxID string) *AccountBuilder {
	cnpjObj, eval := tax.NewCNPJOrCPF(rawTaxID)
	if len(eval) > 0 {
		// Merges CNPJ errors into the Builder's evaluator.
		for field, msg := range eval {
			b.eval.AddFieldError(field, msg)
		}
		return b
	}
	b.taxID = cnpjObj.String()
	return b
}

func (b *AccountBuilder) WithStatus(status ...string) *AccountBuilder {
	if len(status) > 0 {
		b.status = Status(status[0])
		return b
	}
	b.status = StatusActive
	return b
}

func (b *AccountBuilder) WithType(accountType string) *AccountBuilder {
	accountType = strings.ToUpper(accountType)
	t := Type(accountType)
	b.eval.CheckField(t.IsValid(), "type", "invalid account type")
	b.accountType = t
	return b
}

// WithCurrency sets the currency (required).
func (b *AccountBuilder) WithCurrency(currency string) *AccountBuilder {
	currencyUpperCase := strings.ToUpper(currency)
	b.eval.CheckField(validator.NotBlank(currencyUpperCase), "currency", "cannot be blank")

	c := money.Currency(currencyUpperCase)
	b.eval.CheckField(c.IsValid(), "currency", "invalid currency")
	b.currency = c
	return b
}

// WithCreatedAt sets the creation date (optional).
func (b *AccountBuilder) WithCreatedAt(createdAt ...time.Time) *AccountBuilder {
	if len(createdAt) > 0 {
		if !createdAt[0].IsZero() && createdAt[0].After(time.Now()) {
			b.eval.CheckField(false, "created_at", "cannot be in the future")
		}
		b.createdAt = createdAt[0]
		return b
	}
	b.createdAt = time.Now()
	return b
}

// WithUpdatedAt sets the update date (optional).
func (b *AccountBuilder) WithUpdatedAt(updatedAt ...time.Time) *AccountBuilder {
	if len(updatedAt) > 0 {
		if !updatedAt[0].IsZero() && updatedAt[0].After(time.Now()) {
			b.eval.CheckField(false, "updated_at", "cannot be in the future")
		}
		b.updatedAt = updatedAt[0]
		return b
	}

	b.updatedAt = time.Now()
	return b
}

// Build builds and validates the Account.
func (b *AccountBuilder) Build() (*Account, error) {
	b.eval.CheckField(b.balancePolicy.IsValid(), "balance_policy", "invalid balance policy")
	if len(b.eval) > 0 {
		return nil, fault.InvalidEntityError(errors.New("invalid account entity"), b.eval)
	}

	now := time.Now()

	return &Account{
		ID:                b.id,
		AccountExternalID: b.accountExternalID,
		AccountNumber:     b.accountNumber,
		TaxID:             b.taxID,
		Status:            b.status,
		AccountType:       b.accountType,
		BalancePolicy:     b.balancePolicy,
		Currency:          b.currency,
		CreatedAt:         shared.CoalesceTime(b.createdAt, now),
		UpdatedAt:         shared.CoalesceTime(b.updatedAt, now),
	}, nil
}
