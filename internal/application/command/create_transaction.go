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
)

const createTransactionCommand = "CreateTransaction"

type CreateTransaction struct {
	uow                   application.UnitOfWork
	accountRepository     account.Repository
	transactionRepository transaction.Repository
	outboxRepository      outbox.Repository
	tracer                application.Tracer
	log                   application.Logger
	metrics               application.Metrics
}

func NewCreateTransaction(
	uow application.UnitOfWork,
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
	ctx, span := c.tracer.Start(ctx, createTransactionCommand+".Execute")
	defer span.End()
	defer func() {
		c.metrics.RecordCommandDuration(createTransactionCommand, float64(time.Since(start).Milliseconds()))
	}()

	validatedTransaction, err := input.CreateTransactionFacade()
	if err != nil {
		return c.fail(span, err, "invalid input")
	}
	requestFingerprint := validatedTransaction.Fingerprint

	var output *dto.CreateTransactionOutput
	err = c.uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
		output = nil
		entityTransaction, err := input.CreateTransactionFacade()
		if err != nil {
			return err
		}

		existing, err := c.findByIdempotencyKey(txCtx, input.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Fingerprint != requestFingerprint {
				c.metrics.RecordIdempotencyTotal("conflict")
				return fault.IdempotencyConflictError(errors.New("idempotency fingerprint mismatch"))
			}
			c.metrics.RecordIdempotencyTotal("replay")
			output = replayOutput(existing)
			return nil
		}

		debitAccount, creditAccount, err := c.lockAccounts(txCtx, input.DebitAccountID, input.CreditAccountID)
		if err != nil {
			return err
		}
		if err := validateAccounts(debitAccount, creditAccount, entityTransaction.Amount.Currency()); err != nil {
			return err
		}

		assignEntryReferences(entityTransaction, debitAccount, creditAccount)
		if err := entityTransaction.Complete(); err != nil {
			return err
		}
		sortEntries(entityTransaction)

		if err := c.transactionRepository.Save(txCtx, entityTransaction); err != nil {
			return err
		}
		if err := c.saveCreatedEvent(txCtx, entityTransaction, input); err != nil {
			return err
		}

		output = &dto.CreateTransactionOutput{
			TransactionID:    new(entityTransaction.ID.String()),
			Status:           string(entityTransaction.Status),
			IdempotentReplay: false,
		}
		c.metrics.RecordIdempotencyTotal("new")
		return nil
	})
	if err != nil {
		return c.fail(span, err, "transaction failed")
	}

	c.metrics.RecordCommandTotal(createTransactionCommand, commandState(output))
	c.log.InfoJSON("CreateTransaction completed", slog.String("trace_id", span.SpanContext().TraceID()), slog.Bool("idempotent_replay", output.IdempotentReplay))
	return output, nil
}

func (c *CreateTransaction) findByIdempotencyKey(ctx context.Context, key *string) (*transaction.Transaction, error) {
	existing, err := c.transactionRepository.Find(ctx, criteria.TransactionCriteria{IdempotencyKey: key})
	if errors.Is(err, transaction.ErrTransactionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (c *CreateTransaction) lockAccounts(ctx context.Context, debitID, creditID *string) (*account.Account, *account.Account, error) {
	firstID, secondID := debitID, creditID
	if *firstID > *secondID {
		firstID, secondID = secondID, firstID
	}

	first, err := c.accountRepository.FindAccount(ctx, criteria.AccountCriteria{
		AccountExternalID: firstID,
		HasForUpdate:      true,
	})
	if err != nil {
		return nil, nil, err
	}
	second, err := c.accountRepository.FindAccount(ctx, criteria.AccountCriteria{
		AccountExternalID: secondID,
		HasForUpdate:      true,
	})
	if err != nil {
		return nil, nil, err
	}
	if first == nil || second == nil {
		return nil, nil, fault.FindAccountNotFoundError(account.ErrAccountNotFound)
	}

	if first.AccountExternalID == *debitID {
		return first, second, nil
	}
	return second, first, nil
}

func validateAccounts(debit, credit *account.Account, currency money.Currency) error {
	if debit.Status != account.StatusActive || credit.Status != account.StatusActive {
		return fault.FindAccountNotFoundError(account.ErrAccountNotFound)
	}
	if debit.Currency.Mismatch(currency) || credit.Currency.Mismatch(currency) {
		return fault.ErrCurrencyMismatch(errors.New("account currency does not match transaction currency"))
	}
	return nil
}

func assignEntryReferences(entityTransaction *transaction.Transaction, debit, credit *account.Account) {
	for _, entry := range entityTransaction.Entries {
		entry.AddTransnactionID(entityTransaction.ID.String())
		if entry.Direction == transaction.Debit {
			entry.AddAccountID(debit.ID.String())
			continue
		}
		entry.AddAccountID(credit.ID.String())
	}
}

func sortEntries(entityTransaction *transaction.Transaction) {
	sort.SliceStable(entityTransaction.Entries, func(i, j int) bool {
		return entityTransaction.Entries[i].AccountID < entityTransaction.Entries[j].AccountID
	})
}

func replayOutput(existing *transaction.Transaction) *dto.CreateTransactionOutput {
	return &dto.CreateTransactionOutput{
		TransactionID:    new(existing.ID.String()),
		Status:           string(existing.Status),
		IdempotentReplay: true,
	}
}

func (c *CreateTransaction) saveCreatedEvent(ctx context.Context, entityTransaction *transaction.Transaction, input dto.CreateTransactionInput) error {
	newOutbox, err := outbox.NewOutbox(entityTransaction.ID.String(), nil)
	if err != nil {
		return err
	}

	event := transaction.TransactionCreatedFacade(
		*entityTransaction,
		*input.IdempotencyKey,
		*input.DebitAccountID,
		*input.CreditAccountID,
	)
	event.WithEventID(newOutbox.ID.String())
	newOutbox.Payload, err = json.Marshal(event)
	if err != nil {
		return err
	}
	return c.outboxRepository.Save(ctx, newOutbox)
}

func (c *CreateTransaction) fail(span application.Span, err error, reason string) (*dto.CreateTransactionOutput, error) {
	span.RecordError(err)
	c.metrics.RecordCommandTotal(createTransactionCommand, "failure")
	c.log.ErrorJSON("CreateTransaction "+reason, append([]any{
		slog.String("trace_id", span.SpanContext().TraceID()),
	}, fault.Attrs(err)...)...)
	return nil, err
}

func commandState(output *dto.CreateTransactionOutput) string {
	if output.IdempotentReplay {
		return "replay"
	}
	return "success"
}
