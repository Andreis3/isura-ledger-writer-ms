package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

// HoldPolicyProvider supplies an explicitly versioned policy for an eligible account.
type HoldPolicyProvider interface {
	PolicyFor(ctx context.Context, account account.Account, holdType hold.HoldType) (hold.AllocationPolicy, bool, error)
}

type CreateHoldOutput struct {
	Instruction      *hold.Instruction
	IdempotentReplay bool
}

type CreateHold struct {
	uow          application.UnitOfWork
	accounts     account.Repository
	transactions transaction.Repository
	holds        hold.Repository
	policies     HoldPolicyProvider
	outbox       outbox.Repository
}

func NewCreateHold(uow application.UnitOfWork, accounts account.Repository, transactions transaction.Repository, holds hold.Repository, policies HoldPolicyProvider, outboxRepo outbox.Repository) *CreateHold {
	return &CreateHold{uow: uow, accounts: accounts, transactions: transactions, holds: holds, policies: policies, outbox: outboxRepo}
}

func (c *CreateHold) Execute(ctx context.Context, input dto.CreateHoldInput) (*CreateHoldOutput, error) {
	instruction, err := input.CreateHoldFacade()
	if err != nil {
		return nil, err
	}
	requested := instruction.Requested
	var result *CreateHoldOutput
	err = c.uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
		result = nil
		existing, err := c.holds.FindByIdentity(txCtx, input.Origin, input.Reference)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestFingerprint != instruction.RequestFingerprint {
				return hold.ErrHoldConflict
			}
			result = &CreateHoldOutput{Instruction: existing, IdempotentReplay: true}
			return nil
		}
		acc, err := c.accounts.FindAccount(txCtx, criteria.AccountCriteria{ID: &input.AccountID})
		if err != nil {
			return err
		}
		if acc == nil {
			return account.ErrAccountNotFound
		}
		if acc.Currency != requested.Currency() {
			return money.ErrCurrencyMismatch
		}
		var decision hold.HoldDecision
		if acc.HoldsEligible {
			var policy hold.AllocationPolicy
			found := false
			if c.policies != nil {
				policy, found, err = c.policies.PolicyFor(txCtx, *acc, input.Type)
				if err != nil {
					return err
				}
			}
			if found {
				_, balance, err := c.transactions.FindLatestLedgerState(txCtx, acc.ID.String())
				if err != nil {
					return err
				}
				if balance < 0 {
					balance = 0
				}
				funds, err := money.NewMoney(balance, requested.Currency())
				if err != nil {
					return err
				}
				active, err := c.holds.FindEffectiveByAccount(txCtx, acc.ID.String(), requested.Currency())
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
				decisions, err := hold.Evaluate(hold.HoldEvaluationInput{EligibleFunds: funds, ActiveAllocations: allocations, Requests: []hold.HoldRequest{{ID: instruction.ID, Type: instruction.Type, Requested: instruction.Requested}}, Policy: policy})
				if err != nil {
					return err
				}
				decision = decisions[0]
				if err := instruction.ApplyDecision(decision); err != nil {
					return err
				}
			}
		}
		if err := c.holds.Save(txCtx, instruction); err != nil {
			return err
		}
		allocatedDelta := instruction.Allocated
		historyID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		history := hold.History{ID: historyID.String(), HoldID: instruction.ID, EventType: "DECISION_RECORDED", Requested: instruction.Requested, AllocatedDelta: allocatedDelta, Pending: instruction.Pending, PolicyVersion: instruction.PolicyVersion, CreatedAt: time.Now().UTC()}
		if decision.Reason != "" {
			history.Reason = decision.Reason
			history.ConflictIDs = append([]string(nil), decision.ConflictIDs...)
		}
		if err := c.holds.SaveHistory(txCtx, history); err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			HoldID, AccountID, Currency, Status string
			Requested, Allocated, Pending       int64
		}{instruction.ID, instruction.AccountID, instruction.Currency, string(instruction.Status), instruction.Requested.Amount(), instruction.Allocated.Amount(), instruction.Pending.Amount()})
		if err != nil {
			return err
		}
		boxID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		item, err := outbox.NewOutboxBuilder().WithID(boxID.String()).WithAggregateID(instruction.ID).WithAggregateType(outbox.Hold).WithEventType(outbox.HoldEvaluated).WithStatus().WithAttempts().WithCreatedAt().WithPayload(payload).Build()
		if err != nil {
			return err
		}
		if err := c.outbox.Save(txCtx, item); err != nil {
			return err
		}
		result = &CreateHoldOutput{Instruction: instruction}
		return nil
	})
	if err != nil {
		// A concurrent request may win the UNIQUE(origin, reference) insert.
		// Resolve that race after rollback using the same canonical fingerprint.
		existing, lookupErr := c.holds.FindByIdentity(ctx, input.Origin, input.Reference)
		if lookupErr == nil && existing != nil {
			if existing.RequestFingerprint != instruction.RequestFingerprint {
				return nil, hold.ErrHoldConflict
			}
			return &CreateHoldOutput{Instruction: existing, IdempotentReplay: true}, nil
		}
		return nil, fmt.Errorf("create hold: %w", err)
	}
	return result, nil
}
