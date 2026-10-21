package hold

import (
	"errors"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

// ErrInvalidEvaluation indicates that a hold snapshot cannot produce a decision.
var ErrInvalidEvaluation = errors.New("invalid hold evaluation")

const (
	reasonAllocated               = "ALLOCATED_BY_POLICY"
	reasonPartialFunds            = "ELIGIBLE_FUNDS_INSUFFICIENT"
	reasonUnresolvedPriority      = "UNRESOLVED_TYPE_PRIORITY"
	reasonUnresolvedSameType      = "UNRESOLVED_SAME_TYPE_COMPETITION"
	reasonBlockedByUnresolvedHold = "BLOCKED_BY_UNRESOLVED_HOLD"
)

// Evaluate allocates an eligible-funds snapshot according to the supplied policy.
// It does not mutate the input or make decisions about existing allocations.
func Evaluate(input HoldEvaluationInput) ([]HoldDecision, error) {
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidEvaluation, err)
	}

	available, err := availableFunds(input)
	if err != nil {
		return nil, err
	}
	return evaluateRequests(input, available)
}

func availableFunds(input HoldEvaluationInput) (money.Money, error) {
	allocated := int64(0)
	for _, allocation := range input.ActiveAllocations {
		amount := allocation.Allocated.Amount()
		if amount > input.EligibleFunds.Amount()-allocated {
			return money.Money{}, fmt.Errorf("%w: active allocations exceed eligible funds", ErrInvalidEvaluation)
		}
		allocated += amount
	}
	return input.EligibleFunds.Subtract(mustMoney(allocated, input.EligibleFunds.Currency()))
}

func evaluateRequests(input HoldEvaluationInput, available money.Money) ([]HoldDecision, error) {
	decisions := make([]HoldDecision, len(input.Requests))
	indicesByType := make(map[HoldType][]int, len(input.Requests))
	for index, request := range input.Requests {
		indicesByType[request.Type] = append(indicesByType[request.Type], index)
	}
	if hasUnorderedType(input) {
		return evaluateWithUnorderedTypes(input, decisions, available)
	}
	order := policyOrder(input.Policy)
	var unresolved []int
	for _, holdType := range order {
		indices := indicesByType[holdType]
		if len(indices) == 0 {
			continue
		}
		if len(unresolved) > 0 {
			conflicts := append(requestIDs(input, unresolved), requestIDs(input, indices)...)
			for _, index := range indices {
				decisions[index] = newDecision(input, index, zeroMoney(available.Currency()), input.Requests[index].Requested, PendingReview, excludingID(conflicts, input.Requests[index].ID), reasonBlockedByUnresolvedHold)
			}
			continue
		}
		var err error
		available, err = evaluateType(input, indices, decisions, available)
		if err != nil {
			return nil, err
		}
		if len(indices) > 1 && decisions[indices[0]].Status == PendingReview {
			unresolved = append(unresolved, indices...)
		}
	}
	return decisions, nil
}

func evaluateType(input HoldEvaluationInput, indices []int, decisions []HoldDecision, available money.Money) (money.Money, error) {
	requested := int64(0)
	for _, index := range indices {
		amount := input.Requests[index].Requested.Amount()
		if amount > int64(^uint64(0)>>1)-requested {
			return money.Money{}, fmt.Errorf("%w: requested amount overflow", ErrInvalidEvaluation)
		}
		requested += amount
	}
	if requested <= available.Amount() {
		for _, index := range indices {
			decisions[index] = newDecision(input, index, input.Requests[index].Requested, zeroMoney(available.Currency()), Allocated, nil, reasonAllocated)
		}
		return subtractAmount(available, requested)
	}
	if len(indices) == 1 {
		return allocateSingle(input, indices[0], decisions, available)
	}
	ids := requestIDs(input, indices)
	for _, index := range indices {
		decisions[index] = newDecision(input, index, zeroMoney(available.Currency()), input.Requests[index].Requested, PendingReview, excludingID(ids, input.Requests[index].ID), reasonUnresolvedSameType)
	}
	return available, nil
}

func allocateSingle(input HoldEvaluationInput, index int, decisions []HoldDecision, available money.Money) (money.Money, error) {
	request := input.Requests[index]
	if !input.Policy.AllowPartial || available.IsZero() {
		decisions[index] = newDecision(input, index, zeroMoney(available.Currency()), request.Requested, PendingReview, nil, reasonPartialFunds)
		return available, nil
	}
	allocated := available
	pending, err := request.Requested.Subtract(allocated)
	if err != nil {
		return money.Money{}, fmt.Errorf("calculate pending hold amount: %w", err)
	}
	decisions[index] = newDecision(input, index, allocated, pending, PartiallyAllocated, nil, reasonPartialFunds)
	return zeroMoney(available.Currency()), nil
}

func hasUnorderedType(input HoldEvaluationInput) bool {
	for _, request := range input.Requests {
		if !containsType(input.Policy.TypeOrder, request.Type) {
			return true
		}
	}
	return false
}

func evaluateWithUnorderedTypes(input HoldEvaluationInput, decisions []HoldDecision, available money.Money) ([]HoldDecision, error) {
	total := int64(0)
	for _, request := range input.Requests {
		amount := request.Requested.Amount()
		if amount > int64(^uint64(0)>>1)-total {
			return nil, fmt.Errorf("%w: requested amount overflow", ErrInvalidEvaluation)
		}
		total += amount
	}
	if total <= available.Amount() {
		for index, request := range input.Requests {
			if !containsType(input.Policy.TypeOrder, request.Type) {
				decisions[index] = newDecision(input, index, zeroMoney(available.Currency()), request.Requested, PendingReview, nil, reasonUnresolvedPriority)
				continue
			}
			decisions[index] = newDecision(input, index, request.Requested, zeroMoney(available.Currency()), Allocated, nil, reasonAllocated)
		}
		return decisions, nil
	}
	ids := make([]string, 0, len(input.Requests))
	for _, request := range input.Requests {
		ids = append(ids, request.ID)
	}
	for index := range input.Requests {
		reason := reasonUnresolvedPriority
		if containsType(input.Policy.TypeOrder, input.Requests[index].Type) {
			reason = reasonBlockedByUnresolvedHold
		}
		decisions[index] = newDecision(input, index, zeroMoney(available.Currency()), input.Requests[index].Requested, PendingReview, excludingID(ids, input.Requests[index].ID), reason)
	}
	return decisions, nil
}

func newDecision(input HoldEvaluationInput, index int, allocated, pending money.Money, status HoldDecisionStatus, conflicts []string, reason string) HoldDecision {
	return HoldDecision{RequestID: input.Requests[index].ID, PolicyVersion: input.Policy.Version, Requested: input.Requests[index].Requested, Allocated: allocated, Pending: pending, Status: status, ConflictIDs: append([]string(nil), conflicts...), Reason: reason}
}

func policyOrder(policy AllocationPolicy) []HoldType {
	return append([]HoldType(nil), policy.TypeOrder...)
}

func requestIDs(input HoldEvaluationInput, indices []int) []string {
	ids := make([]string, 0, len(indices))
	for _, index := range indices {
		ids = append(ids, input.Requests[index].ID)
	}
	return ids
}

func excludingID(ids []string, excluded string) []string {
	result := make([]string, 0, len(ids)-1)
	for _, id := range ids {
		if id != excluded {
			result = append(result, id)
		}
	}
	return result
}

func containsType(types []HoldType, target HoldType) bool {
	for _, holdType := range types {
		if holdType == target {
			return true
		}
	}
	return false
}

func subtractAmount(value money.Money, amount int64) (money.Money, error) {
	result, err := value.Subtract(mustMoney(amount, value.Currency()))
	if err != nil {
		return money.Money{}, fmt.Errorf("subtract allocated hold funds: %w", err)
	}
	return result, nil
}

func zeroMoney(currency money.Currency) money.Money { return mustMoney(0, currency) }

func mustMoney(amount int64, currency money.Currency) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic("validated hold money became invalid: " + err.Error())
	}
	return value
}
