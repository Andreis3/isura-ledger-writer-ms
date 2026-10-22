package hold

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

// HoldStatus is the persisted lifecycle state of an instruction.
type HoldStatus string

const (
	StatusAllocated          HoldStatus = "ALLOCATED"
	StatusPartiallyAllocated HoldStatus = "PARTIALLY_ALLOCATED"
	StatusPendingReview      HoldStatus = "PENDING_REVIEW"
	StatusReleased           HoldStatus = "RELEASED"
	StatusRejected           HoldStatus = "REJECTED"
)

var (
	ErrInvalidRelease    = errors.New("invalid hold release")
	ErrHoldStateConflict = errors.New("hold state or version conflict")
	ErrInvalidResolution = errors.New("invalid hold resolution")
)

// Instruction is the current persisted state of one idempotently identified hold.
type Instruction struct {
	ID, AccountID, Currency, Origin, Reference string
	Type                                       HoldType
	Requested, Allocated, Pending, Released    money.Money
	Status                                     HoldStatus
	PolicyVersion, RequestFingerprint          string
	Version                                    uint64
	AllowPartial                               bool
}

// ResolutionAction is an explicit operational decision for a pending hold.
type ResolutionAction string

const (
	ActionAllocate          ResolutionAction = "ALLOCATE"
	ActionPartiallyAllocate ResolutionAction = "PARTIALLY_ALLOCATE"
	ActionReject            ResolutionAction = "REJECT"
	ActionKeepPending       ResolutionAction = "KEEP_PENDING"
)

// Resolution carries the values and optimistic concurrency preconditions for a decision.
type Resolution struct {
	Action          ResolutionAction
	ExpectedStatus  HoldStatus
	ExpectedVersion uint64
	Allocated       money.Money
	PolicyVersion   string
}

// Identity returns the idempotency identity shared by an origin and reference.
func (i Instruction) Identity() string {
	return strings.TrimSpace(i.Origin) + ":" + strings.TrimSpace(i.Reference)
}

// Fingerprint returns a stable digest of the immutable instruction content.
func (i Instruction) Fingerprint() string {
	content := strings.Join([]string{i.AccountID, i.Currency, strings.TrimSpace(i.Origin), strings.TrimSpace(i.Reference), string(i.Type), strconv.FormatInt(i.Requested.Amount(), 10)}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

// Validate checks the persistable instruction invariants.
func (i Instruction) Validate() error {
	if i.ID == "" || i.AccountID == "" || strings.TrimSpace(i.Origin) == "" || strings.TrimSpace(i.Reference) == "" || i.Identity() == ":" {
		return fmt.Errorf("%w: instruction identity fields are required", ErrInvalidHold)
	}
	if err := i.Type.Validate(); err != nil {
		return err
	}
	if err := validateMoney(i.Requested, true); err != nil {
		return fmt.Errorf("%w: requested amount: %w", ErrInvalidHold, err)
	}
	for name, value := range map[string]money.Money{"allocated": i.Allocated, "pending": i.Pending, "released": i.Released} {
		if err := validateMoney(value, false); err != nil || value.Currency() != i.Requested.Currency() {
			return fmt.Errorf("%w: invalid %s amount", ErrInvalidHold, name)
		}
	}
	if i.Allocated.Amount() > i.Requested.Amount() || i.Pending.Amount() != i.Requested.Amount()-i.Allocated.Amount() || i.Released.Amount() > i.Allocated.Amount() {
		return fmt.Errorf("%w: amount invariant violated", ErrInvalidHold)
	}
	switch i.Status {
	case StatusAllocated:
		if !i.Allocated.Equal(i.Requested) || i.Released.Equal(i.Allocated) {
			return fmt.Errorf("%w: invalid allocated status", ErrInvalidHold)
		}
	case StatusPartiallyAllocated:
		if !i.Allocated.IsPositive() || !i.Pending.IsPositive() || i.Released.Equal(i.Allocated) {
			return fmt.Errorf("%w: invalid partially allocated status", ErrInvalidHold)
		}
	case StatusPendingReview:
		if i.Allocated.IsPositive() || !i.Released.IsZero() {
			return fmt.Errorf("%w: pending instruction cannot retain funds", ErrInvalidHold)
		}
	case StatusRejected:
		if i.Allocated.IsPositive() || !i.Released.IsZero() {
			return fmt.Errorf("%w: rejected instruction cannot retain or release funds", ErrInvalidHold)
		}
	case StatusReleased:
		if !i.Allocated.IsPositive() || !i.Released.Equal(i.Allocated) {
			return fmt.Errorf("%w: invalid released status", ErrInvalidHold)
		}
	default:
		return fmt.Errorf("%w: invalid status", ErrInvalidHold)
	}
	return nil
}

// Resolve applies an explicit decision to a pending instruction using optimistic concurrency.
func (i *Instruction) Resolve(resolution Resolution) error {
	if i == nil {
		return fmt.Errorf("%w: instruction is required", ErrInvalidResolution)
	}
	if i.Status != StatusPendingReview || resolution.ExpectedStatus != i.Status || resolution.ExpectedVersion != i.Version {
		return ErrHoldStateConflict
	}
	if i.Version == ^uint64(0) {
		return fmt.Errorf("%w: version exhausted", ErrInvalidResolution)
	}
	zero, err := money.NewMoney(0, i.Requested.Currency())
	if err != nil {
		return fmt.Errorf("%w: invalid requested currency: %w", ErrInvalidResolution, err)
	}
	allocated := resolution.Allocated
	if !allocated.Currency().IsValid() || allocated.Currency() != i.Requested.Currency() || allocated.IsNegative() || allocated.Amount() > i.Requested.Amount() {
		return fmt.Errorf("%w: allocated amount must be within the requested amount and currency", ErrInvalidResolution)
	}
	status := StatusPendingReview
	switch resolution.Action {
	case ActionAllocate:
		if !allocated.Equal(i.Requested) {
			return fmt.Errorf("%w: ALLOCATE must allocate the requested amount in full", ErrInvalidResolution)
		}
		status = StatusAllocated
	case ActionPartiallyAllocate:
		if !i.AllowPartial || !allocated.IsPositive() || allocated.Equal(i.Requested) {
			return fmt.Errorf("%w: partial allocation is not permitted or is not partial", ErrInvalidResolution)
		}
		status = StatusPartiallyAllocated
	case ActionReject:
		if !allocated.IsZero() {
			return fmt.Errorf("%w: REJECT cannot allocate funds", ErrInvalidResolution)
		}
		status = StatusRejected
	case ActionKeepPending:
		if !allocated.IsZero() {
			return fmt.Errorf("%w: KEEP_PENDING cannot allocate funds", ErrInvalidResolution)
		}
	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalidResolution, resolution.Action)
	}
	pending, err := i.Requested.Subtract(allocated)
	if err != nil {
		return fmt.Errorf("%w: calculate pending amount: %w", ErrInvalidResolution, err)
	}
	if status == StatusRejected || status == StatusPendingReview {
		allocated, pending = zero, i.Requested
	}
	i.Allocated, i.Pending, i.Released = allocated, pending, zero
	i.Status, i.PolicyVersion = status, resolution.PolicyVersion
	i.Version++
	return nil
}

// ApplyDecision updates the instruction with a domain evaluation result.
func (i *Instruction) ApplyDecision(decision HoldDecision) error {
	if i == nil || decision.RequestID != i.ID || !decision.Requested.Equal(i.Requested) ||
		!decision.Allocated.Currency().IsValid() || !decision.Pending.Currency().IsValid() ||
		decision.Allocated.Currency() != i.Requested.Currency() || decision.Pending.Currency() != i.Requested.Currency() ||
		decision.Allocated.IsNegative() || decision.Pending.IsNegative() ||
		decision.Allocated.Amount() > i.Requested.Amount() || decision.Pending.Amount() != i.Requested.Amount()-decision.Allocated.Amount() {
		return fmt.Errorf("%w: decision does not match instruction", ErrInvalidHold)
	}
	if decision.Status != Allocated && decision.Status != PartiallyAllocated && decision.Status != PendingReview {
		return fmt.Errorf("%w: unknown decision status", ErrInvalidHold)
	}
	if i.Released.Amount() > decision.Allocated.Amount() {
		return fmt.Errorf("%w: decision below released amount", ErrInvalidHold)
	}
	i.Allocated, i.Pending, i.PolicyVersion = decision.Allocated, decision.Pending, decision.PolicyVersion
	switch decision.Status {
	case Allocated, PartiallyAllocated:
		i.Status = HoldStatus(decision.Status)
	case PendingReview:
		i.Status = StatusPendingReview
	}
	if i.Released.Equal(i.Allocated) && i.Allocated.IsPositive() {
		i.Status = StatusReleased
	}
	return nil
}

// Release records an authorized partial or full release, bounded by retained funds.
func (i *Instruction) Release(amount money.Money) error {
	if i == nil || !amount.Currency().IsValid() || amount.Currency() != i.Allocated.Currency() || !amount.IsPositive() {
		return fmt.Errorf("%w: amount must be positive and use the hold currency", ErrInvalidRelease)
	}
	remaining := i.Allocated.Amount() - i.Released.Amount()
	if remaining < 0 || amount.Amount() > remaining {
		return fmt.Errorf("%w: amount exceeds retained balance", ErrInvalidRelease)
	}
	released, err := i.Released.Add(amount)
	if err != nil {
		return fmt.Errorf("%w: add released amount: %w", ErrInvalidRelease, err)
	}
	i.Released = released
	if i.Released.Equal(i.Allocated) {
		i.Status = StatusReleased
	}
	return nil
}

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

// HoldEvaluationInput is the caller-supplied snapshot used for one allocation evaluation.
type HoldEvaluationInput struct {
	// EligibleFunds must already reflect the caller's eligibility assessment.
	// Evaluation applies the operational allocation policy; it does not determine
	// legal eligibility or account for guarantees and restrictions itself.
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
	allocationIDs := make(map[string]struct{}, len(in.ActiveAllocations))
	for _, allocation := range in.ActiveAllocations {
		if err := allocation.Validate(); err != nil {
			return fmt.Errorf("%w: allocation %q: %w", ErrInvalidSnapshot, allocation.HoldID, err)
		}
		if _, exists := allocationIDs[allocation.HoldID]; exists {
			return fmt.Errorf("%w: duplicate active allocation hold ID %q", ErrInvalidSnapshot, allocation.HoldID)
		}
		if allocation.Allocated.Currency() != in.EligibleFunds.Currency() {
			return fmt.Errorf("%w: allocation %q currency differs from eligible funds", ErrInvalidSnapshot, allocation.HoldID)
		}
		allocationIDs[allocation.HoldID] = struct{}{}
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

// HoldDecision is the result value of one hold evaluation. Its exported fields
// are mutable by the caller; Evaluate returns independently owned conflict ID
// slices and never mutates previously returned decisions.
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
