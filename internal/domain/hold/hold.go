package hold

import (
	"errors"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

var (
	ErrInvalidHoldType = errors.New("invalid hold type")
	ErrInvalidHold     = errors.New("invalid hold")
	ErrInvalidSnapshot = errors.New("invalid hold evaluation snapshot")
)

// HoldType identifies the declared origin or purpose of a hold instruction.
type HoldType string

const (
	Judicial    HoldType = "JUDICIAL"
	Regulatory  HoldType = "REGULATORY"
	Guarantee   HoldType = "GUARANTEE"
	Contractual HoldType = "CONTRACTUAL"
	Operational HoldType = "OPERATIONAL"
)

func (t HoldType) IsValid() bool {
	switch t {
	case Judicial, Regulatory, Guarantee, Contractual, Operational:
		return true
	default:
		return false
	}
}

// Validate verifies that the hold type is one of the supported values.
func (t HoldType) Validate() error {
	if !t.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidHoldType, t)
	}
	return nil
}

// HoldRequest is a request to reserve eligible funds.
type HoldRequest struct {
	ID        string
	Type      HoldType
	Requested money.Money
}

func (r HoldRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: request ID is required", ErrInvalidHold)
	}
	if err := r.Type.Validate(); err != nil {
		return fmt.Errorf("%w: request type: %w", ErrInvalidHold, err)
	}
	if err := validateMoney(r.Requested, true); err != nil {
		return fmt.Errorf("%w: requested amount: %w", ErrInvalidHold, err)
	}
	return nil
}

// ActiveAllocation is the portion already reserved by an existing hold.
type ActiveAllocation struct {
	HoldID    string
	Type      HoldType
	Allocated money.Money
}

func (a ActiveAllocation) Validate() error {
	if a.HoldID == "" {
		return fmt.Errorf("%w: active allocation hold ID is required", ErrInvalidHold)
	}
	if err := a.Type.Validate(); err != nil {
		return fmt.Errorf("%w: active allocation type: %w", ErrInvalidHold, err)
	}
	if err := validateMoney(a.Allocated, false); err != nil {
		return fmt.Errorf("%w: allocated amount: %w", ErrInvalidHold, err)
	}
	return nil
}

// HoldEvaluationInput is the immutable snapshot used for one allocation evaluation.
type HoldEvaluationInput struct {
	EligibleFunds     money.Money
	ActiveAllocations []ActiveAllocation
	Requests          []HoldRequest
	Policy            AllocationPolicy
}

func (in HoldEvaluationInput) Validate() error {
	if err := validateMoney(in.EligibleFunds, false); err != nil {
		return fmt.Errorf("%w: eligible funds: %w", ErrInvalidSnapshot, err)
	}
	if err := in.Policy.Validate(); err != nil {
		return fmt.Errorf("%w: policy: %w", ErrInvalidSnapshot, err)
	}
	requestIDs := make(map[string]struct{}, len(in.Requests))
	for _, request := range in.Requests {
		if err := request.Validate(); err != nil {
			return fmt.Errorf("%w: request %q: %w", ErrInvalidSnapshot, request.ID, err)
		}
		if request.Requested.Currency() != in.EligibleFunds.Currency() {
			return fmt.Errorf("%w: request %q currency differs from eligible funds", ErrInvalidSnapshot, request.ID)
		}
		if _, exists := requestIDs[request.ID]; exists {
			return fmt.Errorf("%w: duplicate request ID %q", ErrInvalidSnapshot, request.ID)
		}
		requestIDs[request.ID] = struct{}{}
	}
	for _, allocation := range in.ActiveAllocations {
		if err := allocation.Validate(); err != nil {
			return fmt.Errorf("%w: allocation %q: %w", ErrInvalidSnapshot, allocation.HoldID, err)
		}
		if allocation.Allocated.Currency() != in.EligibleFunds.Currency() {
			return fmt.Errorf("%w: allocation %q currency differs from eligible funds", ErrInvalidSnapshot, allocation.HoldID)
		}
	}
	return nil
}

// HoldDecisionStatus is the outcome of evaluating a hold request.
type HoldDecisionStatus string

const (
	Allocated          HoldDecisionStatus = "ALLOCATED"
	PartiallyAllocated HoldDecisionStatus = "PARTIALLY_ALLOCATED"
	PendingReview      HoldDecisionStatus = "PENDING_REVIEW"
)

// HoldDecision is the immutable result of one hold evaluation.
type HoldDecision struct {
	RequestID     string
	PolicyVersion string
	Requested     money.Money
	Allocated     money.Money
	Pending       money.Money
	Status        HoldDecisionStatus
	ConflictIDs   []string
	Reason        string
}

func validateMoney(value money.Money, mustBePositive bool) error {
	if !value.Currency().IsValid() {
		return fmt.Errorf("invalid currency %q", value.Currency())
	}
	if value.IsNegative() || (mustBePositive && !value.IsPositive()) {
		return errors.New("amount has an invalid sign")
	}
	return nil
}
