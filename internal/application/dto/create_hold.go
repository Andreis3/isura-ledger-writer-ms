package dto

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

type CreateHoldInput struct {
	AccountID, Currency, Origin, Reference string
	Type                                   hold.HoldType
	Requested                              money.Money
}

// ReleaseHoldInput is an internally authorized request to release retained funds.
type ReleaseHoldInput struct {
	HoldID, IdempotencyKey, AuthorizationReference string
	Amount                                         money.Money
}

func (d CreateHoldInput) CreateHoldFacade() (*hold.Instruction, error) {
	id, err := entity.NewIDV7()
	if err != nil {
		return nil, err
	}
	zero, err := money.NewMoney(0, d.Requested.Currency())
	if err != nil {
		return nil, err
	}
	instruction := &hold.Instruction{
		ID: id.String(), AccountID: d.AccountID, Currency: d.Currency,
		Origin: d.Origin, Reference: d.Reference, Type: d.Type,
		Requested: d.Requested, Allocated: zero, Pending: d.Requested,
		Released: zero, Status: hold.StatusPendingReview,
		Version: 1,
	}
	instruction.RequestFingerprint = instruction.Fingerprint()
	if err := instruction.Validate(); err != nil {
		return nil, err
	}
	if instruction.Currency != string(instruction.Requested.Currency()) {
		return nil, money.ErrCurrencyMismatch
	}
	return instruction, nil
}
