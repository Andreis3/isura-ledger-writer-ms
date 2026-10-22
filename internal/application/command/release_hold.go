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
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
)

// ReleaseHoldOutput describes a newly applied or replayed release.
type ReleaseHoldOutput struct {
	Instruction      *hold.Instruction
	Release          *hold.Release
	IdempotentReplay bool
}

// ReleaseHold applies an authorized release within the ledger unit of work.
type ReleaseHold struct {
	uow    application.UnitOfWork
	holds  hold.Repository
	outbox outbox.Repository
}

// NewReleaseHold constructs the release command.
func NewReleaseHold(uow application.UnitOfWork, holds hold.Repository, outboxRepo outbox.Repository) *ReleaseHold {
	return &ReleaseHold{uow: uow, holds: holds, outbox: outboxRepo}
}

// Execute applies a partial or total release, preserving idempotency and audit data.
func (c *ReleaseHold) Execute(ctx context.Context, input dto.ReleaseHoldInput) (*ReleaseHoldOutput, error) {
	if c == nil || c.uow == nil || c.holds == nil || c.outbox == nil ||
		strings.TrimSpace(input.HoldID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" ||
		strings.TrimSpace(input.AuthorizationReference) == "" || !input.Amount.Currency().IsValid() || !input.Amount.IsPositive() {
		return nil, hold.ErrInvalidRelease
	}
	fingerprint := releaseFingerprint(input)
	var result *ReleaseHoldOutput
	err := c.uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
		result = nil
		existingRelease, err := c.holds.FindRelease(txCtx, input.IdempotencyKey)
		if err != nil {
			return err
		}
		if existingRelease != nil {
			if existingRelease.RequestFingerprint != fingerprint {
				return hold.ErrReleaseConflict
			}
			instruction, err := c.holds.FindByID(txCtx, existingRelease.HoldID)
			if err != nil {
				return err
			}
			if instruction == nil {
				return fmt.Errorf("find hold for release replay: %w", hold.ErrInvalidRelease)
			}
			result = &ReleaseHoldOutput{Instruction: instruction, Release: existingRelease, IdempotentReplay: true}
			return nil
		}

		instruction, err := c.holds.FindByID(txCtx, input.HoldID)
		if err != nil {
			return err
		}
		if instruction == nil {
			return fmt.Errorf("hold %q not found: %w", input.HoldID, hold.ErrInvalidRelease)
		}
		if err := instruction.Release(input.Amount); err != nil {
			return err
		}
		now := time.Now().UTC()
		releaseID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		release := hold.Release{ID: releaseID.String(), HoldID: input.HoldID,
			IdempotencyKey: input.IdempotencyKey, AuthorizationReference: input.AuthorizationReference,
			RequestFingerprint: fingerprint, Amount: input.Amount, CreatedAt: now}
		if err := c.holds.SaveRelease(txCtx, release); err != nil {
			return err
		}
		if err := c.holds.Update(txCtx, instruction); err != nil {
			return err
		}
		historyID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		zero, err := money.NewMoney(0, input.Amount.Currency())
		if err != nil {
			return err
		}
		history := hold.History{ID: historyID.String(), HoldID: instruction.ID, EventType: "RELEASE_RECORDED",
			Requested: instruction.Requested, AllocatedDelta: zero, Pending: instruction.Pending,
			ReleasedDelta: input.Amount, IdempotencyKey: input.IdempotencyKey, Reason: input.AuthorizationReference, CreatedAt: now}
		if err := c.holds.SaveHistory(txCtx, history); err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			HoldID, AccountID, Currency, Status string
			Amount, Released, Remaining         int64
		}{instruction.ID, instruction.AccountID, instruction.Currency, string(instruction.Status), input.Amount.Amount(), instruction.Released.Amount(), instruction.Allocated.Amount() - instruction.Released.Amount()})
		if err != nil {
			return err
		}
		boxID, err := entity.NewIDV7()
		if err != nil {
			return err
		}
		item, err := outbox.NewOutboxBuilder().WithID(boxID.String()).WithAggregateID(instruction.ID).
			WithAggregateType(outbox.Hold).WithEventType(outbox.HoldReleased).WithStatus().WithAttempts().
			WithCreatedAt(now).WithPayload(payload).Build()
		if err != nil {
			return err
		}
		if err := c.outbox.Save(txCtx, item); err != nil {
			return err
		}
		result = &ReleaseHoldOutput{Instruction: instruction, Release: &release}
		return nil
	})
	if err == nil {
		return result, nil
	}
	if errors.Is(err, hold.ErrReleaseConflict) {
		return nil, err
	}
	// Resolve a concurrent insert of the same idempotency key after its transaction commits.
	existing, lookupErr := c.holds.FindRelease(ctx, input.IdempotencyKey)
	if lookupErr == nil && existing != nil {
		if existing.RequestFingerprint != fingerprint {
			return nil, hold.ErrReleaseConflict
		}
		instruction, findErr := c.holds.FindByID(ctx, existing.HoldID)
		if findErr == nil && instruction != nil {
			return &ReleaseHoldOutput{Instruction: instruction, Release: existing, IdempotentReplay: true}, nil
		}
	}
	return nil, fmt.Errorf("release hold: %w", err)
}

func releaseFingerprint(input dto.ReleaseHoldInput) string {
	content := strings.Join([]string{input.HoldID, input.IdempotencyKey, input.AuthorizationReference,
		string(input.Amount.Currency()), strconv.FormatInt(input.Amount.Amount(), 10)}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}
