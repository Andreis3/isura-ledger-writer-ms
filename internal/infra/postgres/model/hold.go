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
	Version                                                         uint64
	AllowPartial                                                    bool
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
	version := instruction.Version
	if version == 0 {
		version = 1
	}
	return Hold{
		ID: instruction.ID, AccountID: instruction.AccountID,
		Currency: string(instruction.Requested.Currency()), Origin: instruction.Origin,
		Reference: instruction.Reference, Type: string(instruction.Type),
		RequestedAmount: instruction.Requested.Amount(), AllocatedAmount: instruction.Allocated.Amount(),
		PendingAmount: instruction.Pending.Amount(), ReleasedAmount: instruction.Released.Amount(),
		Version: version, AllowPartial: instruction.AllowPartial,
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
		Version:            value.Version, AllowPartial: value.AllowPartial,
	}
	if err := instruction.Validate(); err != nil {
		return nil, err
	}
	return instruction, nil
}

// HoldDecision is the PostgreSQL representation of an immutable resolution.
type HoldDecision struct {
	ID, HoldID, IdempotencyKey, RequestFingerprint  string
	AuthorizationReference, AuthorizedBy            string
	Action, PolicyVersion, Reason, ResultStatus     string
	PolicySnapshot                                  []byte
	ExpectedVersion                                 uint64
	RequestedAmount, ResultAllocated, ResultPending int64
	CreatedAt                                       time.Time
}

// ToHoldDecisionModel maps a validated decision to its persistence shape.
func ToHoldDecisionModel(value hold.Decision) (HoldDecision, error) {
	if value.ID == "" || value.HoldID == "" || value.IdempotencyKey == "" || len(value.RequestFingerprint) != 64 ||
		value.AuthorizationReference == "" || value.AuthorizedBy == "" || value.Reason == "" ||
		value.ExpectedVersion == 0 || len(value.PolicySnapshot) == 0 || value.Requested.Amount() <= 0 ||
		!value.Requested.Currency().IsValid() || !value.ResultAllocated.Currency().IsValid() ||
		value.ResultAllocated.Currency() != value.Requested.Currency() || !value.ResultPending.Currency().IsValid() ||
		value.ResultPending.Currency() != value.Requested.Currency() || value.ResultAllocated.IsNegative() ||
		value.ResultPending.IsNegative() || value.ResultAllocated.Amount()+value.ResultPending.Amount() != value.Requested.Amount() {
		return HoldDecision{}, fmt.Errorf("invalid hold decision")
	}
	return HoldDecision{ID: value.ID, HoldID: value.HoldID, IdempotencyKey: value.IdempotencyKey,
		RequestFingerprint: value.RequestFingerprint, AuthorizationReference: value.AuthorizationReference,
		AuthorizedBy: value.AuthorizedBy, Action: string(value.Action), PolicyVersion: value.PolicyVersion,
		Reason: value.Reason, ResultStatus: value.ResultStatus, PolicySnapshot: value.PolicySnapshot,
		ExpectedVersion: value.ExpectedVersion, RequestedAmount: value.Requested.Amount(),
		ResultAllocated: value.ResultAllocated.Amount(), ResultPending: value.ResultPending.Amount(),
		CreatedAt: value.CreatedAt}, nil
}

// ToHoldDecisionDomain maps the persisted decision to domain money values.
func ToHoldDecisionDomain(value HoldDecision, currency money.Currency) (*hold.Decision, error) {
	requested, err := money.NewMoney(value.RequestedAmount, currency)
	if err != nil {
		return nil, err
	}
	allocated, err := money.NewMoney(value.ResultAllocated, currency)
	if err != nil {
		return nil, err
	}
	pending, err := money.NewMoney(value.ResultPending, currency)
	if err != nil {
		return nil, err
	}
	return &hold.Decision{ID: value.ID, HoldID: value.HoldID, IdempotencyKey: value.IdempotencyKey,
		RequestFingerprint: value.RequestFingerprint, AuthorizationReference: value.AuthorizationReference,
		AuthorizedBy: value.AuthorizedBy, Action: hold.ResolutionAction(value.Action),
		PolicyVersion: value.PolicyVersion, Reason: value.Reason, ResultStatus: value.ResultStatus,
		PolicySnapshot: value.PolicySnapshot, ExpectedVersion: value.ExpectedVersion,
		Requested: requested, ResultAllocated: allocated, ResultPending: pending, CreatedAt: value.CreatedAt}, nil
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
