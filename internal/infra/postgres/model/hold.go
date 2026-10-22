package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

// Hold is the PostgreSQL representation of a current hold instruction.
type Hold struct {
	ID, AccountID, Currency, Origin, Reference, Type                string
	RequestedAmount, AllocatedAmount, PendingAmount, ReleasedAmount int64
	Status, PolicyVersion, RequestFingerprint                       string
	CreatedAt, UpdatedAt                                            time.Time
}

// ToHoldModel maps a validated domain instruction to its persistence shape.
func ToHoldModel(instruction *hold.Instruction) (Hold, error) {
	if instruction == nil {
		return Hold{}, fmt.Errorf("hold instruction is nil")
	}
	if err := instruction.Validate(); err != nil {
		return Hold{}, err
	}
	return Hold{
		ID: instruction.ID, AccountID: instruction.AccountID,
		Currency: string(instruction.Requested.Currency()), Origin: instruction.Origin,
		Reference: instruction.Reference, Type: string(instruction.Type),
		RequestedAmount: instruction.Requested.Amount(), AllocatedAmount: instruction.Allocated.Amount(),
		PendingAmount: instruction.Pending.Amount(), ReleasedAmount: instruction.Released.Amount(),
		Status: string(instruction.Status), PolicyVersion: instruction.PolicyVersion,
		RequestFingerprint: instruction.RequestFingerprint,
	}, nil
}

// ToHoldDomain maps persisted state back into the validated domain aggregate.
func ToHoldDomain(value Hold) (*hold.Instruction, error) {
	currency := money.Currency(value.Currency)
	requested, err := money.NewMoney(value.RequestedAmount, currency)
	if err != nil {
		return nil, err
	}
	allocated, err := money.NewMoney(value.AllocatedAmount, currency)
	if err != nil {
		return nil, err
	}
	pending, err := money.NewMoney(value.PendingAmount, currency)
	if err != nil {
		return nil, err
	}
	released, err := money.NewMoney(value.ReleasedAmount, currency)
	if err != nil {
		return nil, err
	}
	instruction := &hold.Instruction{
		ID: value.ID, AccountID: value.AccountID, Currency: value.Currency,
		Origin: value.Origin, Reference: value.Reference, Type: hold.HoldType(value.Type),
		Requested: requested, Allocated: allocated, Pending: pending, Released: released,
		Status: hold.HoldStatus(value.Status), PolicyVersion: value.PolicyVersion,
		RequestFingerprint: value.RequestFingerprint,
	}
	if err := instruction.Validate(); err != nil {
		return nil, err
	}
	return instruction, nil
}

// HoldHistory is the PostgreSQL representation of an append-only audit event.
type HoldHistory struct {
	ID, HoldID, EventType                                         string
	RequestedAmount, AllocatedDelta, PendingAmount, ReleasedDelta int64
	PolicyVersion, IdempotencyKey, Reason                         *string
	ConflictIDs                                                   []byte
	CreatedAt                                                     time.Time
}

// ConflictIDsJSON encodes history conflict identifiers for PostgreSQL jsonb.
func ConflictIDsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(ids)
}
