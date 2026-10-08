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
	"github.com/andreis3/isura-ledger-ms/internal/application/service"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/jackc/pgx/v5/pgconn"
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
	maxEntries            int
}

func NewCreateTransaction(
	uow application.UnitOfWork,
	accountRepository account.Repository,
	transactionRepository transaction.Repository,
	outboxRepository outbox.Repository,
	tracer application.Tracer,
	log application.Logger,
	metrics application.Metrics,
	maxEntries int,
) *CreateTransaction {
	if maxEntries <= 0 {
		maxEntries = application.DefaultMaxTransactionEntries
	}
	return &CreateTransaction{
		uow:                   uow,
		accountRepository:     accountRepository,
		transactionRepository: transactionRepository,
		outboxRepository:      outboxRepository,
		tracer:                tracer,
		log:                   log,
		metrics:               metrics,
		maxEntries:            maxEntries,
	}
}

func (c *CreateTransaction) Execute(ctx context.Context, input dto.CreateTransactionInput) (*dto.CreateTransactionOutput, error) {
	start := time.Now()
	ctx, span := c.tracer.Start(ctx, createTransactionCommand+".Execute")
	defer span.End()
	defer func() {
		c.metrics.RecordCommandDuration(createTransactionCommand, float64(time.Since(start).Milliseconds()))
	}()
	if requestedEntryCount(input) > c.maxEntries {
		err := fault.InvalidEntityError(transaction.ErrInvalidMaxEntries, map[string]any{
			"entries": "maximum entries exceeded",
		})
		return c.fail(span, err, "invalid input")
	}

	validatedTransaction, err := input.CreateTransactionFacade()
	if err != nil {
		return c.fail(span, err, "invalid input")
	}
	requestFingerprint := validatedTransaction.Fingerprint

	var output *dto.CreateTransactionOutput
	err = c.uow.WithRetryableTransaction(ctx, func(txCtx context.Context) error {
		output = nil
		var attemptErr error
		output, attemptErr = c.executeWithinTransaction(txCtx, input, requestFingerprint)
		return attemptErr
	})
	if err != nil {
		replay, replayErr := c.replayAfterIdempotencyRace(ctx, input, requestFingerprint, err)
		if replayErr != nil {
			return c.fail(span, replayErr, "transaction failed")
		}
		output = replay
	}
	if output == nil {
		return c.fail(span, err, "transaction failed")
	}
	if output.IdempotentReplay {
		c.metrics.RecordIdempotencyTotal("replay")
	} else {
		c.metrics.RecordIdempotencyTotal("new")
	}

	c.metrics.RecordCommandTotal(createTransactionCommand, commandState(output))
	c.log.InfoJSON("CreateTransaction completed", slog.String("trace_id", span.SpanContext().TraceID()), slog.Bool("idempotent_replay", output.IdempotentReplay))
	return output, nil
}

func (c *CreateTransaction) executeWithinTransaction(
	ctx context.Context,
	input dto.CreateTransactionInput,
	fingerprint string,
) (*dto.CreateTransactionOutput, error) {
	// Rebuild on every attempt so retries recalculate from fresh persisted state.
	entityTransaction, err := input.CreateTransactionFacade()
	if err != nil {
		return nil, err
	}
	existing, err := c.findByIdempotencyKey(ctx, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Fingerprint != fingerprint {
			c.metrics.RecordIdempotencyTotal("conflict")
			return nil, fault.IdempotencyConflictError(errors.New("idempotency fingerprint mismatch"))
		}
		return replayOutput(existing), nil
	}
	if err := c.persistNewTransaction(ctx, input, entityTransaction); err != nil {
		return nil, err
	}
	return &dto.CreateTransactionOutput{
		TransactionID:    new(entityTransaction.ID.String()),
		Status:           string(entityTransaction.Status),
		IdempotentReplay: false,
	}, nil
}

func (c *CreateTransaction) persistNewTransaction(
	ctx context.Context,
	input dto.CreateTransactionInput,
	entityTransaction *transaction.Transaction,
) error {
	accountsByExternalID, accounts, err := c.loadAccounts(ctx, entityTransaction.Entries)
	if err != nil {
		return err
	}
	if err := validateEntryAccounts(entityTransaction.Entries, accountsByExternalID); err != nil {
		return err
	}
	if input.Entries == nil {
		debitAccount := accountsByExternalID[debitAccountID(input)]
		creditAccount := accountsByExternalID[creditAccountID(input)]
		if debitAccount.ID == creditAccount.ID {
			return fault.InvalidTransferError(transaction.ErrSameAccountTransfer)
		}
	}

	assignEntryReferences(entityTransaction, accountsByExternalID)
	if err := service.AssignLedgerEntries(ctx, c.transactionRepository, entityTransaction, accounts...); err != nil {
		return err
	}
	if err := entityTransaction.Complete(); err != nil {
		return err
	}
	if err := c.transactionRepository.Save(ctx, entityTransaction); err != nil {
		return err
	}
	return c.saveCreatedEvent(ctx, entityTransaction, input)
}

func requestedEntryCount(input dto.CreateTransactionInput) int {
	if input.Entries == nil {
		return 2
	}
	return len(input.Entries)
}

func debitAccountID(input dto.CreateTransactionInput) string {
	if input.DebitAccountID == nil {
		return ""
	}
	return *input.DebitAccountID
}

func creditAccountID(input dto.CreateTransactionInput) string {
	if input.CreditAccountID == nil {
		return ""
	}
	return *input.CreditAccountID
}

func (c *CreateTransaction) replayAfterIdempotencyRace(
	ctx context.Context,
	input dto.CreateTransactionInput,
	fingerprint string,
	err error,
) (*dto.CreateTransactionOutput, error) {
	if !isIdempotencyUniqueViolation(err) {
		return nil, err
	}

	existing, findErr := c.findByIdempotencyKey(ctx, input.IdempotencyKey)
	if findErr != nil {
		return nil, findErr
	}
	if existing == nil {
		return nil, err
	}
	if existing.Fingerprint != fingerprint {
		c.metrics.RecordIdempotencyTotal("conflict")
		return nil, fault.IdempotencyConflictError(errors.New("idempotency fingerprint mismatch"))
	}
	return replayOutput(existing), nil
}

func isIdempotencyUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == "idx_transactions_idempotency_key" ||
		pgErr.ConstraintName == "transactions_idempotency_key_key"
}

func (c *CreateTransaction) findByIdempotencyKey(ctx context.Context, key *string) (*transaction.Transaction, error) {
	existing, err := c.transactionRepository.Find(ctx, transaction.TransactionCriteria{IdempotencyKey: key})
	if errors.Is(err, transaction.ErrTransactionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (c *CreateTransaction) loadAccounts(ctx context.Context, entries []*transaction.Entry) (map[string]*account.Account, []*account.Account, error) {
	externalIDs := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.AccountExternalID]; exists {
			continue
		}
		seen[entry.AccountExternalID] = struct{}{}
		externalIDs = append(externalIDs, entry.AccountExternalID)
	}
	sort.Strings(externalIDs)

	accountsByExternalID := make(map[string]*account.Account, len(externalIDs))
	accounts := make([]*account.Account, 0, len(externalIDs))
	for _, externalID := range externalIDs {
		loaded, err := c.accountRepository.FindAccount(ctx, criteria.AccountCriteria{AccountExternalID: &externalID})
		if err != nil {
			return nil, nil, err
		}
		if loaded == nil {
			return nil, nil, fault.FindAccountNotFoundError(account.ErrAccountNotFound)
		}
		accountsByExternalID[externalID] = loaded
		accounts = append(accounts, loaded)
	}
	return accountsByExternalID, accounts, nil
}

func validateEntryAccounts(entries []*transaction.Entry, accountsByExternalID map[string]*account.Account) error {
	for _, entry := range entries {
		loaded, exists := accountsByExternalID[entry.AccountExternalID]
		if !exists || loaded.Status != account.StatusActive {
			return fault.FindAccountNotFoundError(account.ErrAccountNotFound)
		}
		if loaded.Currency.Mismatch(entry.Amount.Currency()) {
			return fault.ErrCurrencyMismatch(errors.New("account currency does not match entry currency"))
		}
	}
	return nil
}

func assignEntryReferences(entityTransaction *transaction.Transaction, accountsByExternalID map[string]*account.Account) {
	for _, entry := range entityTransaction.Entries {
		entry.AssignTransactionID(entityTransaction.ID.String())
		if loaded, exists := accountsByExternalID[entry.AccountExternalID]; exists {
			entry.AssignAccountID(loaded.ID.String())
		}
	}
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
		debitAccountID(input),
		creditAccountID(input),
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
	c.metrics.RecordCommandTotal(createTransactionCommand, commandFailureState(err))
	c.log.ErrorJSON("CreateTransaction "+reason, append([]any{
		slog.String("trace_id", span.SpanContext().TraceID()),
	}, fault.Attrs(err)...)...)
	return nil, err
}

func commandFailureState(err error) string {
	if errors.Is(err, fault.ErrInsufficientBalance) {
		return "insufficient_balance"
	}
	return "failure"
}

func commandState(output *dto.CreateTransactionOutput) string {
	if output.IdempotentReplay {
		return "replay"
	}
	return "success"
}
