package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/application/security"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

const resolvePendingHoldCommand = "ResolvePendingHold"

// ResolvePendingHold records an authenticated resolution as a single UoW with history and outbox.
type ResolvePendingHold struct {
	uow          application.UnitOfWork
	accounts     account.Repository
	transactions transaction.Repository
	holds        hold.Repository
	policies     HoldPolicyProvider
	outbox       outbox.Repository
	tracer       application.Tracer
	log          application.Logger
	metrics      application.Metrics
}

func NewResolvePendingHold(uow application.UnitOfWork, accounts account.Repository, transactions transaction.Repository,
	holds hold.Repository, policies HoldPolicyProvider, outboxRepo outbox.Repository,
	tracer application.Tracer, log application.Logger, metrics application.Metrics) *ResolvePendingHold {
	return &ResolvePendingHold{uow: uow, accounts: accounts, transactions: transactions, holds: holds,
		policies: policies, outbox: outboxRepo, tracer: tracer, log: log, metrics: metrics}
}

func (c *ResolvePendingHold) Execute(ctx context.Context, input dto.ResolvePendingHoldInput) (output *dto.ResolvePendingHoldOutput, resultErr error) {
	if c == nil {
		return nil, hold.ErrInvalidResolution
	}
	started := time.Now()
	ctx, span := c.tracer.Start(ctx, resolvePendingHoldCommand+".Execute")
	defer func() {
		state, replay := "success", false
		if output != nil {
			replay = output.IdempotentReplay
		}
		observeHoldCommand(ctx, started, span, c.log, c.metrics, resolvePendingHoldCommand, state, replay, resultErr)
	}()
	principal, ok := security.PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(principal.Subject) == "" || !principal.Allows(security.ResolveFundsHold) {
		return nil, ErrUnauthorizedHoldResolution
	}
	if c.uow == nil || c.accounts == nil || c.transactions == nil || c.holds == nil || c.outbox == nil ||
		strings.TrimSpace(input.HoldID) == "" || strings.TrimSpace(input.DecisionID) == "" ||
		strings.TrimSpace(input.AuthorizationReference) == "" || strings.TrimSpace(input.Reason) == "" || input.ExpectedVersion == 0 {
		return nil, hold.ErrInvalidResolution
	}
	if input.Action == hold.ActionAllocate || input.Action == hold.ActionPartiallyAllocate {
		if !input.RequestedAmount.IsPositive() || !input.RequestedAmount.Currency().IsValid() || c.policies == nil || strings.TrimSpace(input.PolicyVersion) == "" {
			return nil, hold.ErrInvalidResolution
		}
	} else if (input.Action != hold.ActionReject && input.Action != hold.ActionKeepPending) || input.RequestedAmount.IsPositive() || input.PolicyVersion != "" {
		return nil, hold.ErrInvalidResolution
	}
	fingerprint := resolveHoldFingerprint(input, principal.Subject)
	var result *dto.ResolvePendingHoldOutput
	err := c.uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
		result = nil
		existing, err := c.holds.FindDecision(txCtx, input.DecisionID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return hold.ErrDecisionConflict
			}
			instruction, err := c.holds.FindByID(txCtx, existing.HoldID)
			if err != nil {
				return err
			}
			if instruction == nil {
				return hold.ErrInvalidResolution
			}
			result = &dto.ResolvePendingHoldOutput{Instruction: instruction, Decision: existing, IdempotentReplay: true}
			return nil
		}
		instruction, err := c.holds.FindByID(txCtx, input.HoldID)
		if err != nil {
			return err
		}
		if instruction == nil {
			return hold.ErrInvalidResolution
		}
		if instruction.Status != hold.StatusPendingReview || instruction.Version != input.ExpectedVersion {
			return hold.ErrHoldStateConflict
		}
		zero, err := money.NewMoney(0, instruction.Requested.Currency())
		if err != nil {
			return err
		}
		allocated := zero
		var policySnapshot []byte
		if input.Action == hold.ActionAllocate || input.Action == hold.ActionPartiallyAllocate {
			if input.RequestedAmount.Currency() != instruction.Requested.Currency() || input.RequestedAmount.Amount() > instruction.Pending.Amount() {
				return hold.ErrInvalidResolution
			}
			if input.Action == hold.ActionAllocate && !input.RequestedAmount.Equal(instruction.Pending) {
				return hold.ErrInvalidResolution
			}
			acc, err := c.accounts.FindAccount(txCtx, criteria.AccountCriteria{ID: &instruction.AccountID})
			if err != nil {
				return err
			}
			if acc == nil {
				return account.ErrAccountNotFound
			}
			if !acc.HoldsEligible || string(acc.Currency) != instruction.Currency {
				return hold.ErrInvalidResolution
			}
			policy, found, err := c.policies.PolicyFor(txCtx, *acc, instruction.Type)
			if err != nil {
				return err
			}
			if !found || policy.Version != input.PolicyVersion {
				return hold.ErrInvalidResolution
			}
			if err := policy.Validate(); err != nil {
				return err
			}
			policySnapshot, err = json.Marshal(policy)
			if err != nil {
				return err
			}
			_, balance, err := c.transactions.FindLatestLedgerState(txCtx, acc.ID.String())
			if err != nil {
				return err
			}
			if balance < 0 {
				balance = 0
			}
			funds, err := money.NewMoney(balance, instruction.Requested.Currency())
			if err != nil {
				return err
			}
			active, err := c.holds.FindEffectiveByAccount(txCtx, acc.ID.String(), instruction.Requested.Currency())
			if err != nil {
				return err
			}
			allocations := make([]hold.ActiveAllocation, 0, len(active))
			for _, current := range active {
				remaining, err := current.Allocated.Subtract(current.Released)
				if err != nil {
					return err
				}
				allocations = append(allocations, hold.ActiveAllocation{HoldID: current.ID, Type: current.Type, Allocated: remaining})
			}
			policyForDecision := policy
			policyForDecision.AllowPartial = policy.AllowPartial && instruction.AllowPartial
			decisions, err := hold.Evaluate(hold.HoldEvaluationInput{EligibleFunds: funds, ActiveAllocations: allocations,
				Requests: []hold.HoldRequest{{ID: instruction.ID, Type: instruction.Type, Requested: input.RequestedAmount}}, Policy: policyForDecision})
			if err != nil {
				return err
			}
			allocated = decisions[0].Allocated
			if input.Action == hold.ActionAllocate && !allocated.Equal(input.RequestedAmount) {
				return hold.ErrInvalidResolution
			}
			if input.Action == hold.ActionPartiallyAllocate && (!instruction.AllowPartial || !policy.AllowPartial || !allocated.IsPositive()) {
				return hold.ErrInvalidResolution
			}
		} else {
			policySnapshot = []byte(`{}`)
		}
		if err := instruction.Resolve(hold.Resolution{Action: input.Action, ExpectedStatus: hold.StatusPendingReview,
			ExpectedVersion: input.ExpectedVersion, Allocated: allocated, PolicyVersion: input.PolicyVersion}); err != nil {
			return err
		}
		now := time.Now().UTC()
		decisionID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		decision := hold.Decision{ID: decisionID.String(), HoldID: instruction.ID, IdempotencyKey: input.DecisionID,
			RequestFingerprint: fingerprint, AuthorizationReference: input.AuthorizationReference, AuthorizedBy: principal.Subject,
			Action: input.Action, PolicyVersion: input.PolicyVersion, Reason: input.Reason, ResultStatus: string(instruction.Status),
			PolicySnapshot: policySnapshot, ExpectedVersion: input.ExpectedVersion, Requested: instruction.Requested,
			ResultAllocated: instruction.Allocated, ResultPending: instruction.Pending, CreatedAt: now}
		if err := c.holds.SaveDecision(txCtx, decision); err != nil {
			return err
		}
		if err := c.holds.Update(txCtx, instruction); err != nil {
			return err
		}
		historyID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		if err := c.holds.SaveHistory(txCtx, hold.History{ID: historyID.String(), HoldID: instruction.ID, EventType: "DECISION_RESOLVED",
			Requested: instruction.Requested, AllocatedDelta: instruction.Allocated, Pending: instruction.Pending, ReleasedDelta: zero,
			PolicyVersion: input.PolicyVersion, IdempotencyKey: input.DecisionID, Reason: input.Reason, CreatedAt: now}); err != nil {
			return err
		}
		boxID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			EventID         string                `json:"event_id"`
			EventVersion    int                   `json:"event_version"`
			OccurredAt      time.Time             `json:"occurred_at"`
			DecisionID      string                `json:"decision_id"`
			HoldID          string                `json:"hold_id"`
			AccountID       string                `json:"account_id"`
			Action          hold.ResolutionAction `json:"action"`
			Status          hold.HoldStatus       `json:"status"`
			RequestedAmount int64                 `json:"requested_amount"`
			AllocatedAmount int64                 `json:"allocated_amount"`
			PendingAmount   int64                 `json:"pending_amount"`
			Currency        string                `json:"currency"`
			PolicyVersion   *string               `json:"policy_version"`
		}{boxID.String(), 1, now, input.DecisionID, instruction.ID, instruction.AccountID, input.Action, instruction.Status,
			input.RequestedAmount.Amount(), instruction.Allocated.Amount(), instruction.Pending.Amount(), instruction.Currency, optionalPolicyVersion(input.PolicyVersion)})
		if err != nil {
			return err
		}
		item, err := outbox.NewOutboxBuilder().WithID(boxID.String()).WithAggregateID(instruction.ID).WithAggregateType(outbox.Hold).
			WithEventType(outbox.HoldDecisionResolved).WithStatus().WithAttempts().WithCreatedAt(now).WithPayload(payload).Build()
		if err != nil {
			return err
		}
		if err := c.outbox.Save(txCtx, item); err != nil {
			return err
		}
		result = &dto.ResolvePendingHoldOutput{Instruction: instruction, Decision: &decision}
		return nil
	})
	if err == nil {
		return result, nil
	}
	// A unique-key race or uncertain commit is resolved by reading the durable decision.
	existing, lookupErr := c.holds.FindDecision(ctx, input.DecisionID)
	if lookupErr == nil && existing != nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, hold.ErrDecisionConflict
		}
		instruction, findErr := c.holds.FindByID(ctx, existing.HoldID)
		if findErr == nil && instruction != nil {
			return &dto.ResolvePendingHoldOutput{Instruction: instruction, Decision: existing, IdempotentReplay: true}, nil
		}
	}
	if errors.Is(err, hold.ErrDecisionConflict) {
		return nil, err
	}
	return nil, fmt.Errorf("resolve pending hold: %w", err)
}

var ErrUnauthorizedHoldResolution = errors.New("caller is not authorized to resolve funds holds")

func resolveHoldFingerprint(input dto.ResolvePendingHoldInput, subject string) string {
	fields := []string{input.HoldID, input.DecisionID, input.AuthorizationReference, subject, string(input.Action),
		input.PolicyVersion, strconv.FormatUint(input.ExpectedVersion, 10), input.Reason,
		string(input.RequestedAmount.Currency()), strconv.FormatInt(input.RequestedAmount.Amount(), 10)}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(fields, "\x00"))))
}

func optionalPolicyVersion(version string) *string {
	if version == "" {
		return nil
	}
	return &version
}
