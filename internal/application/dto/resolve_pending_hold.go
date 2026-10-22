package dto

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

// ResolvePendingHoldInput contains the decision fields; caller identity is read from security context.
type ResolvePendingHoldInput struct {
	HoldID, DecisionID, AuthorizationReference, PolicyVersion, Reason string
	Action                                                            hold.ResolutionAction
	RequestedAmount                                                   money.Money
	ExpectedVersion                                                   uint64
}

type ResolvePendingHoldOutput struct {
	Instruction      *hold.Instruction
	Decision         *hold.Decision
	IdempotentReplay bool
}
