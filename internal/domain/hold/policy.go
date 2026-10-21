package hold

import (
	"errors"
	"fmt"
)

var ErrInvalidAllocationPolicy = errors.New("invalid allocation policy")

// UnresolvedConflictRule defines how a policy handles competition it cannot resolve.
type UnresolvedConflictRule string

const PendingReviewOnConflict UnresolvedConflictRule = "PENDING_REVIEW"

// AllocationPolicy is an explicitly supplied, versioned operational policy.
type AllocationPolicy struct {
	Version            string
	TypeOrder          []HoldType
	AllowPartial       bool
	UnresolvedConflict UnresolvedConflictRule
}

func (p AllocationPolicy) Validate() error {
	if p.Version == "" {
		return fmt.Errorf("%w: version is required", ErrInvalidAllocationPolicy)
	}
	if len(p.TypeOrder) == 0 {
		return fmt.Errorf("%w: type order is required", ErrInvalidAllocationPolicy)
	}
	seen := make(map[HoldType]struct{}, len(p.TypeOrder))
	for _, holdType := range p.TypeOrder {
		if err := holdType.Validate(); err != nil {
			return fmt.Errorf("%w: type order: %w", ErrInvalidAllocationPolicy, err)
		}
		if _, exists := seen[holdType]; exists {
			return fmt.Errorf("%w: duplicate type %q in type order", ErrInvalidAllocationPolicy, holdType)
		}
		seen[holdType] = struct{}{}
	}
	if p.UnresolvedConflict != PendingReviewOnConflict {
		return fmt.Errorf("%w: unsupported unresolved conflict rule %q", ErrInvalidAllocationPolicy, p.UnresolvedConflict)
	}
	return nil
}
