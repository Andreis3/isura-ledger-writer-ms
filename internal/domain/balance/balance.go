package balance

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/shared"
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

type Balance struct {
	id        entity.ID
	accountID string
	amount    money.Money
	createdAT time.Time
	updatedAT time.Time
}

type BalanceBuilder struct {
	ID        entity.ID
	AccountID string
	Amount    money.Money
	CreatedAT time.Time
	UpdatedAT time.Time
	eval      validator.Evaluator
}

func NewBalanceBuilder() *BalanceBuilder {
	return &BalanceBuilder{}
}

func (b *BalanceBuilder) WithID(id ...string) *BalanceBuilder {
	if len(id) > 0 {
		b.eval.CheckField(validator.NotBlank(id[0]), "id", "cannot be blank")
		b.eval.CheckField(validator.MatchesUUID(id[0]), "id", "is not uuid")
		uuidV7, err := entity.NewID(id[0])
		if err != nil {
			b.eval.AddFieldError("id", err.Error())
		}

		b.ID = uuidV7
		return b
	}

	uuidv7, err := entity.NewIDV7()
	if err != nil {
		b.eval.AddFieldError("id", err.Error())
	}
	b.ID = uuidv7
	return b
}

func (b *BalanceBuilder) WithAccountID(accountID string) *BalanceBuilder {
	b.eval.CheckField(validator.NotBlank(accountID), "account_id", "cannot be blank")
	b.eval.CheckField(validator.MatchesUUIDv7(accountID), "account_id", "is not uuid")
	b.AccountID = accountID
	return b
}

func (b *BalanceBuilder) WithAmount(amount int64, currency money.Currency) *BalanceBuilder {
	m, err := money.NewMoney(amount, currency)
	if err != nil {
		b.eval.AddFieldError("amount", err.Error())
	}
	b.Amount = m
	return b
}

func (b *BalanceBuilder) WithCreatedAt(createdAt ...time.Time) *BalanceBuilder {
	if len(createdAt) > 0 {
		if !createdAt[0].IsZero() && createdAt[0].After(time.Now()) {
			b.eval.CheckField(false, "created_at", "cannot be in the future")
		}
		b.CreatedAT = createdAt[0]
		return b
	}
	b.UpdatedAT = time.Now()
	return b
}

func (b *BalanceBuilder) WithUpdatedAt(updatedAt ...time.Time) *BalanceBuilder {
	if len(updatedAt) > 0 {
		if !updatedAt[0].IsZero() && updatedAt[0].After(time.Now()) {
			b.eval.CheckField(false, "updated_at", "cannot be in the future")
		}
		b.UpdatedAT = updatedAt[0]
		return b
	}
	b.UpdatedAT = time.Now()
	return b
}

func (b *BalanceBuilder) Build() (*Balance, error) {
	now := time.Now()

	return &Balance{
		id:        b.ID,
		accountID: b.AccountID,
		amount:    b.Amount,
		createdAT: shared.CoalesceTime(b.CreatedAT, now),
		updatedAT: shared.CoalesceTime(b.UpdatedAT, now),
	}, nil
}

func (b *Balance) ID() entity.ID {
	return b.id
}

func (b *Balance) AccountID() string {
	return b.accountID
}

func (b *Balance) Amount() money.Money {
	return b.amount
}

func (b *Balance) CreatedAT() time.Time {
	return b.createdAT
}

func (b *Balance) UpdatedAT() time.Time {
	return b.updatedAT
}
