package command

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue"
)

type CreateBalance struct {
	balanceRepository balance.Repository
	accountRepository account.Repository
	log               application.Logger
	tracer            application.Tracer
	metrics           application.Metrics
}

func NewCreateBalance(
	balanceRepository balance.Repository,
	accountRepository account.Repository,
	log application.Logger,
	tracer application.Tracer,
	metrics application.Metrics,
) *CreateBalance {
	return &CreateBalance{
		balanceRepository: balanceRepository,
		accountRepository: accountRepository,
		log:               log,
		tracer:            tracer,
		metrics:           metrics,
	}
}

func (c *CreateBalance) Execute(ctx context.Context, input dto.CreateBalanceInput) error {
	start := time.Now()
	ctx, span := c.tracer.Start(ctx, "CreateBalance.Execute")
	tracerID := span.SpanContext().TraceID()
	defer span.End()
	defer c.metrics.RecordCommandDuration("CreateBalance", float64(time.Since(start).Milliseconds()))

	c.log.InfoJSON("CreateBalance received request",
		slog.String("trace_id", tracerID),
		slog.Any("input", input),
	)

	balanceEntity, err := c.validate(input)
	if err != nil {
		span.RecordError(err)
		c.log.CriticalJSON("CreateBalance failed to validate",
			append([]any{
				slog.String("trace_id", tracerID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateBalance", "failure")
		// TODO: criar um erro de dominino
		return queue.NewPermanentError(err)
	}

	existBalance, err := c.balanceRepository.Find(ctx, criteria.BalanceCriteria{
		AccountID: new(balanceEntity.AccountID()),
	})

	if err != nil && !errors.Is(err, balance.ErrBalanceNotFound) {
		c.log.CriticalJSON("CreateBalance failed to find balance by account ID",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_id", input.AccountID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateBalance", "failure")
		return err
	}

	if existBalance != nil {
		c.log.InfoJSON("CreateBalance balance already exists",
			slog.String("trace_id", tracerID),
			slog.String("account_id", balanceEntity.AccountID()),
		)
		c.metrics.RecordCommandTotal("CreateBalance", "exist")
		return nil
	}

	parmsCriteria := criteria.AccountCriteria{
		ID: new(balanceEntity.AccountID()),
	}

	existing, err := c.accountRepository.FindAccount(ctx, parmsCriteria)
	if err != nil {
		if errors.Is(err, account.ErrAccountNotFound) {
			c.log.CriticalJSON("CreateBalance account not found",
				append([]any{
					slog.String("trace_id", tracerID),
					slog.String("account_id", input.AccountID)},
					fault.Attrs(err)...)...,
			)
			span.RecordError(err)
			c.metrics.RecordCommandTotal("CreateBalance", "failure")
			return err // Ou retorne um erro de domínio apropriado
		}

		c.log.CriticalJSON("CreateBalance failed to find account by account ID",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_id", input.AccountID)},
				fault.Attrs(err)...)...,
		)
		span.RecordError(err)
		c.metrics.RecordCommandTotal("CreateBalance", "failure")
		return err
	}

	if existing == nil {
		c.log.CriticalJSON("CreateBalance account not found",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_id", input.AccountID)},
			))
		span.RecordError(err)
		c.metrics.RecordCommandTotal("CreateBalance", "failure")
		return fault.FindAccountNotFoundError(errors.New("account not found"))
	}

	c.log.InfoJSON("CreateBalance account found successfully",
		slog.String("trace_id", tracerID),
		slog.String("account_id", existing.ID.String()),
	)

	err = c.balanceRepository.Save(ctx, balanceEntity)
	if err != nil {
		c.log.CriticalJSON("Error creating new balance sheet",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_id", balanceEntity.ID().String())},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateBalance", "failure")
		return err
	}

	c.log.InfoJSON("Balance create with success",
		slog.String("trace_id", tracerID),
		slog.String("id", balanceEntity.ID().String()),
		slog.String("account_id", balanceEntity.AccountID()),
	)

	return nil
}

func (c *CreateBalance) validate(input dto.CreateBalanceInput) (*balance.Balance, error) {
	return input.NewBalanceDomain()
}
