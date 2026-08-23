package command

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

type CreateAccount struct {
	accountRepository account.Repository
	natsPublisher     event.Publisher
	log               application.Logger
	tracer            application.Tracer
	metrics           application.Metrics
}

func NewCreateAccount(
	accountRepository account.Repository,
	natsPublisher event.Publisher,
	log application.Logger,
	tracer application.Tracer,
	metrics application.Metrics,
) *CreateAccount {
	return &CreateAccount{
		accountRepository: accountRepository,
		natsPublisher:     natsPublisher,
		log:               log,
		tracer:            tracer,
		metrics:           metrics,
	}
}

func (c *CreateAccount) Execute(ctx context.Context, input dto.CreateAccountInput) (*dto.CreateAccountOutput, error) {
	start := time.Now()
	ctx, span := c.tracer.Start(ctx, "CreateAccount.Execute")
	tracerID := span.SpanContext().TraceID()
	defer span.End()
	defer c.metrics.RecordCommandDuration("CreateAccount", float64(time.Since(start).Milliseconds()))

	c.log.InfoJSON("CreateAccount received request",
		slog.String("trace_id", tracerID),
		slog.Any("input", input),
	)

	accountEntity, err := c.validate(input)
	if err != nil {
		span.RecordError(err)
		c.log.CriticalJSON("CreateAccount failed to validate",
			append([]any{
				slog.String("trace_id", tracerID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateAccount", "failure")
		return nil, err
	}

	parmsCriteria := criteria.AccountCriteria{
		AccountExternalID: &accountEntity.AccountExternalID,
		Currency:          new(string(accountEntity.Currency)),
		Type:              new(string(accountEntity.AccountType)),
	}

	existing, err := c.accountRepository.FindAccount(ctx, parmsCriteria)
	if err != nil && !errors.Is(err, account.ErrAccountNotFound) {
		c.log.CriticalJSON("CreateAccount failed to find account by external ID",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_external_id", accountEntity.AccountExternalID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateAccount", "failure")
		return nil, err
	}

	if existing != nil {
		c.log.InfoJSON("CreateAccount account already exists",
			slog.String("trace_id", tracerID),
			slog.String("account_external_id", accountEntity.AccountExternalID),
		)
		c.metrics.RecordCommandTotal("CreateAccount", "exist")
		return &dto.CreateAccountOutput{
			AccountID: new(existing.ID.String()),
		}, nil
	}

	if err != nil {
		c.log.CriticalJSON("CreateAccount failed to create account entity",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_external_id", input.AccountExternalID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateAccount", "failure")
		return nil, err
	}

	err = c.accountRepository.Save(ctx, accountEntity)
	if err != nil {
		c.log.CriticalJSON("CreateAccount failed to save account",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_external_id", accountEntity.AccountExternalID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateAccount", "failure")
		return nil, err
	}

	c.log.InfoJSON("CreateAccount account created successfully",
		slog.String("trace_id", tracerID),
		slog.String("account_id", accountEntity.ID.String()),
		slog.String("account_external_id", accountEntity.AccountExternalID),
	)

	eventAccount := event.NewCreateBalanceEvent(accountEntity.ID.String(), string(accountEntity.Currency))

	err = c.natsPublisher.Publish(ctx, eventAccount)
	if err != nil {
		c.log.CriticalJSON("CreateAccount failed to publish",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("account_external_id", accountEntity.AccountExternalID)},
				fault.Attrs(err)...)...,
		)
		c.metrics.RecordCommandTotal("CreateAccount", "failure")
		return nil, err
	}

	return &dto.CreateAccountOutput{
		AccountID: new(accountEntity.ID.String()),
	}, nil
}

func (c *CreateAccount) validate(input dto.CreateAccountInput) (*account.Account, error) {

	return input.CreateAccountFacade()
}
