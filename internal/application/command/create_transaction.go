package command

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/andreis3/isura-ledger-ms/internal/util"
)

type CreateTransaction struct {
	uow                   application.UnitOfWork
	accountRepository     account.Repository
	transactionRepository transaction.Repository
	outboxRepository      outbox.Repository
	tracer                application.Tracer
	log                   application.Logger
	metrics               application.Metrics
}

func NewCreateTransaction(uow application.UnitOfWork,
	accountRepository account.Repository,
	transactionRepository transaction.Repository,
	outboxRepository outbox.Repository,
	tracer application.Tracer,
	log application.Logger,
	metrics application.Metrics,
) *CreateTransaction {
	return &CreateTransaction{
		uow:                   uow,
		accountRepository:     accountRepository,
		transactionRepository: transactionRepository,
		outboxRepository:      outboxRepository,
		tracer:                tracer,
		log:                   log,
		metrics:               metrics,
	}
}

func (c *CreateTransaction) Execute(ctx context.Context, input dto.CreateTransactionInput) (*dto.CreateTransactionOutput, error) {
	start := time.Now()
	ctx, span := c.tracer.Start(ctx, "CreateTransaction.Execute")
	tracerID := span.SpanContext().TraceID()
	defer span.End()
	defer func() {
		c.metrics.RecordCommandDuration("CreateTransaction.Execute", float64(time.Since(start).Milliseconds()))
	}()

	c.log.InfoJSON("CreateTransaction received request",
		slog.String("trace_id", tracerID),
		slog.Any("input", input),
	)

	entityTransaction, err := input.CreateTransactionFacade()
	if err != nil {
		c.log.CriticalJSON("CreateTransaction failed due to invalid input",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("idempotency_key", util.String(input.IdempotencyKey))},
				fault.Attrs(err)...)...,
		)
		span.RecordError(err)
		return nil, err
	}

	existingTransaction, err := c.transactionRepository.Find(ctx, criteria.TransactionCriteria{
		IdempotencyKey: input.IdempotencyKey,
	})
	if err != nil && !errors.Is(err, transaction.ErrTransactionNotFound) {
		c.log.CriticalJSON("CreateTransaction failed to find transaction by idempotency key",
			append([]any{
				slog.String("trace_id", tracerID),
				slog.String("idempotency_key", *input.IdempotencyKey)},
				fault.Attrs(err)...)...,
		)
		span.RecordError(err)
		return nil, err
	}

	if existingTransaction != nil {
		c.log.InfoJSON("CreateTransaction transaction already exists",
			slog.String("trace_id", tracerID),
			slog.String("transaction_id", existingTransaction.ID.String()),
		)

		return &dto.CreateTransactionOutput{
			TransactionID: new(existingTransaction.ID.String()),
		}, nil
	}

	var output *dto.CreateTransactionOutput

	errUow := c.uow.WithTransaction(ctx, func(ctxTx context.Context) error {
		// 1. Deterministic lock to avoid deadlock
		firstID, secondID := input.DebitAccountID, input.CreditAccountID
		if *firstID > *secondID {
			firstID, secondID = secondID, firstID
		}

		// 2. Search with SELECT FOR UPDATE from TX
		// Here we guarantee that the balance read is the "last truth" and no one touches it until the commit
		_, err := c.accountRepository.FindAccount(ctxTx, criteria.AccountCriteria{
			AccountExternalID: firstID,
			HasForUpdate:      true,
		})
		if err != nil {
			span.RecordError(err)
			return err
		}

		_, err = c.accountRepository.FindAccount(ctxTx, criteria.AccountCriteria{
			AccountExternalID: secondID,
			HasForUpdate:      true,
		})
		if err != nil {
			span.RecordError(err)
			return err
		}

		// 3. Search for the complete objects for domain logic
		debitAccount, err := c.accountRepository.FindAccount(ctxTx, criteria.AccountCriteria{
			AccountExternalID: input.DebitAccountID,
		})
		if err != nil {
			span.RecordError(err)
			return err
		}

		creditAccount, err := c.accountRepository.FindAccount(ctxTx, criteria.AccountCriteria{
			AccountExternalID: input.CreditAccountID,
		})
		if err != nil {
			span.RecordError(err)
			return err
		}

		if debitAccount == nil {
			span.RecordError(account.ErrAccountNotFound)
			return account.ErrAccountNotFound
		}

		if creditAccount == nil {
			span.RecordError(account.ErrAccountNotFound)
			return account.ErrAccountNotFound
		}

		txCurrency := entityTransaction.Amount.Currency()

		validateCurrency := func(accountType string, accCurrency money.Currency) error {
			if accCurrency.Mismatch(txCurrency) {
				errMismatch := errors.New("currency mismatch: " + accountType + " currency " + string(accCurrency) + " does not match transaction currency " + string(txCurrency))
				domainErr := fault.ErrCurrencyMismatch(errMismatch)
				span.RecordError(domainErr)
				c.log.CriticalJSON("CreateTransaction failed due to currency mismatch",
					append([]any{
						slog.String("trace_id", tracerID)},
						fault.Attrs(domainErr)...)...,
				)
				return domainErr
			}
			return nil
		}

		if err := validateCurrency("debit account", debitAccount.Currency); err != nil {
			return err
		}
		if err := validateCurrency("credit account", creditAccount.Currency); err != nil {
			return err
		}

		// Associate internal account ID and transaction ID by iterating through entries
		for _, entry := range entityTransaction.Entries {
			entry.AddTransnactionID(entityTransaction.ID.String())

			switch entry.Direction {
			case transaction.Debit:
				entry.AddAccountID(debitAccount.ID.String())
			case transaction.Credit:
				entry.AddAccountID(creditAccount.ID.String())
			}
		}

		// 4. Strict sorting of entries using the internal ID to align with Postgres FK
		sort.Slice(entityTransaction.Entries, func(i, j int) bool {
			return entityTransaction.Entries[i].AccountID < entityTransaction.Entries[j].AccountID
		})

		err = c.transactionRepository.Save(ctxTx, entityTransaction)
		if err != nil {
			span.RecordError(err)
			return err
		}

		payload, err := json.Marshal(transaction.TransactionCreatedFacade(*entityTransaction, *input.IdempotencyKey, *input.DebitAccountID, *input.CreditAccountID))
		if err != nil {
			span.RecordError(err)
			return err
		}

		newOutbox, err := outbox.NewOutbox(
			string(entityTransaction.ID.String()),
			payload,
		)
		if err != nil {
			span.RecordError(err)
			return err
		}

		err = c.outboxRepository.Save(ctxTx, newOutbox)
		if err != nil {
			span.RecordError(err)
			return err
		}

		output = &dto.CreateTransactionOutput{
			TransactionID: new(entityTransaction.ID.String()),
		}

		return nil
	})

	if errUow != nil {
		span.RecordError(errUow)
		return nil, errUow
	}

	return output, nil
}
