//go:build unit
// +build unit

package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	debitExternalID  = "d290f1ee-6c54-4b01-90e6-d701748f0851"
	creditExternalID = "a290f1ee-6c54-4b01-90e6-d701748f0852"
)

var _ = Describe("CreateTransaction", func() {
	It("creates a completed transfer and its outbox event atomically", func() {
		input := validInput()
		accounts := newAccountRepository()
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{}
		sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(string(transaction.Completed)))
		Expect(result.IdempotentReplay).To(BeFalse())
		Expect(transactions.saved).NotTo(BeNil())
		Expect(transactions.saved.Status).To(Equal(transaction.Completed))
		Expect(transactions.saved.Entries).To(HaveLen(2))
		Expect(outboxes.saved).NotTo(BeNil())
		Expect(accounts.findCalls).To(Equal([]string{creditExternalID, debitExternalID}))

		var event transaction.TransactionCreated
		Expect(json.Unmarshal(outboxes.saved.Payload, &event)).To(Succeed())
		Expect(event.EventID).NotTo(BeEmpty())
		Expect(event.Status).To(Equal(string(transaction.Completed)))
		Expect(event.Metadata).To(Equal(input.Metadata))
	})

	It("rejects a missing account without persisting anything", func() {
		accounts := newAccountRepository()
		delete(accounts.accounts, creditExternalID)
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{}
		sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

		result, err := sut.Execute(context.Background(), validInput())

		Expect(result).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(transactions.saved).To(BeNil())
		Expect(outboxes.saved).To(BeNil())
	})

	It("returns the persisted transaction on an identical idempotent replay", func() {
		input := validInput()
		persisted, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		Expect(persisted.Complete()).To(Succeed())
		accounts := newAccountRepository()
		transactions := &transactionRepository{existing: persisted}
		sut := newCommand(accounts, transactions, &outboxRepository{}, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.TransactionID).To(Equal(new(persisted.ID.String())))
		Expect(result.Status).To(Equal(string(transaction.Completed)))
		Expect(result.IdempotentReplay).To(BeTrue())
		Expect(transactions.saved).To(BeNil())
		Expect(transactions.findCriteria.IdempotencyKey).NotTo(BeNil())
		Expect(*transactions.findCriteria.IdempotencyKey).To(Equal(*input.IdempotencyKey))
	})

	It("rejects reuse of an idempotency key with a different fingerprint", func() {
		input := validInput()
		persisted, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		persisted.Fingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		accounts := newAccountRepository()
		transactions := &transactionRepository{existing: persisted}
		sut := newCommand(accounts, transactions, &outboxRepository{}, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(result).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("idempotency fingerprint mismatch")))
		Expect(transactions.saved).To(BeNil())
	})

	It("returns no output when the outbox write fails and the unit of work rolls back", func() {
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{saveErr: errors.New("outbox unavailable")}
		sut := newCommand(newAccountRepository(), transactions, outboxes, &unitOfWork{})

		result, err := sut.Execute(context.Background(), validInput())

		Expect(result).To(BeNil())
		Expect(err).To(MatchError("outbox unavailable"))
	})

	It("retries the complete operation after a sequence conflict", func() {
		transactions := &transactionRepository{saveErrors: []error{
			&pgconn.PgError{Code: "23505", ConstraintName: "unique_entry_sequence_number"},
		}}
		uow := &retryingUnitOfWork{}
		accounts := newAccountRepository()
		sut := newCommand(accounts, transactions, &outboxRepository{}, uow)

		result, err := sut.Execute(context.Background(), validInput())

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IdempotentReplay).To(BeFalse())
		Expect(transactions.saveCalls).To(Equal(2))
		Expect(accounts.findCalls).To(Equal([]string{
			creditExternalID, debitExternalID,
			creditExternalID, debitExternalID,
		}))
	})

	It("replays after a concurrent request wins the idempotency race", func() {
		input := validInput()
		persisted, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		Expect(persisted.Complete()).To(Succeed())
		transactions := &transactionRepository{
			saveErrors:     []error{&pgconn.PgError{Code: "23505", ConstraintName: "idx_transactions_idempotency_key"}},
			existingOnSave: persisted,
		}
		sut := newCommand(newAccountRepository(), transactions, &outboxRepository{}, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.TransactionID).To(Equal(new(persisted.ID.String())))
		Expect(result.IdempotentReplay).To(BeTrue())
	})

	It("rejects a concurrent idempotency race with a different fingerprint", func() {
		input := validInput()
		persisted, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		persisted.Fingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		transactions := &transactionRepository{
			saveErrors:     []error{&pgconn.PgError{Code: "23505", ConstraintName: "idx_transactions_idempotency_key"}},
			existingOnSave: persisted,
		}
		sut := newCommand(newAccountRepository(), transactions, &outboxRepository{}, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(result).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("idempotency fingerprint mismatch")))
		Expect(transactions.saved).To(BeNil())
	})
})

func validInput() dto.CreateTransactionInput {
	key := "transfer-application-test"
	amount := int64(150000)
	currency := string(money.BRL)
	operation := string(transaction.OperationTransfer)
	debit := debitExternalID
	credit := creditExternalID
	return dto.CreateTransactionInput{
		IdempotencyKey:  &key,
		DebitAccountID:  &debit,
		CreditAccountID: &credit,
		Amount:          &amount,
		Currency:        &currency,
		Operation:       &operation,
		Metadata:        map[string]string{"source": "unit-test"},
	}
}

type unitOfWork struct{}

func (u *unitOfWork) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (u *unitOfWork) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type retryingUnitOfWork struct{}

func (u *retryingUnitOfWork) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (u *retryingUnitOfWork) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return fn(ctx)
	}
	return nil
}

type accountRepository struct {
	accounts  map[string]*account.Account
	findCalls []string
}

func newAccountRepository() *accountRepository {
	debitID, _ := entity.NewID("019ff448-c43d-70d3-83c7-dfa0674469b7")
	creditID, _ := entity.NewID("019ff448-c43d-70d3-83c7-dfa0674469b8")
	return &accountRepository{accounts: map[string]*account.Account{
		debitExternalID:  {ID: debitID, AccountExternalID: debitExternalID, Status: account.StatusActive, Currency: money.BRL},
		creditExternalID: {ID: creditID, AccountExternalID: creditExternalID, Status: account.StatusActive, Currency: money.BRL},
	}}
}

func (r *accountRepository) Save(context.Context, *account.Account) error { return nil }

func (r *accountRepository) FindAccount(_ context.Context, params criteria.AccountCriteria) (*account.Account, error) {
	if params.AccountExternalID == nil {
		return nil, nil
	}
	r.findCalls = append(r.findCalls, *params.AccountExternalID)
	return r.accounts[*params.AccountExternalID], nil
}

type transactionRepository struct {
	existing       *transaction.Transaction
	existingOnSave *transaction.Transaction
	saved          *transaction.Transaction
	findCriteria   transaction.TransactionCriteria
	saveErrors     []error
	saveCalls      int
}

func (r *transactionRepository) Save(_ context.Context, value *transaction.Transaction) error {
	r.saveCalls++
	if len(r.saveErrors) > 0 {
		err := r.saveErrors[0]
		r.saveErrors = r.saveErrors[1:]
		if r.existingOnSave != nil {
			r.existing = r.existingOnSave
		}
		return err
	}
	r.saved = value
	return nil
}

func (r *transactionRepository) Find(_ context.Context, params transaction.TransactionCriteria) (*transaction.Transaction, error) {
	r.findCriteria = params
	if r.existing == nil {
		return nil, transaction.ErrTransactionNotFound
	}
	return r.existing, nil
}

func (r *transactionRepository) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	return r.existing != nil, nil
}

type outboxRepository struct {
	saved   *outbox.Outbox
	saveErr error
}

func (r *outboxRepository) Save(_ context.Context, value *outbox.Outbox) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = value
	return nil
}

func (r *outboxRepository) ClaimPending(context.Context, int, int, time.Duration) ([]*outbox.Outbox, error) {
	return nil, nil
}

func (r *outboxRepository) FindAll(context.Context, outbox.StatusOutbox, int) ([]*outbox.Outbox, error) {
	return nil, nil
}

func (r *outboxRepository) UpdateOutboxData(context.Context, entity.ID, outbox.UpdateOutboxData) error {
	return nil
}

func newCommand(accounts *accountRepository, transactions *transactionRepository, outboxes *outboxRepository, uow application.UnitOfWork) *command.CreateTransaction {
	return command.NewCreateTransaction(uow, accounts, transactions, outboxes, testTracer{}, testLogger{}, testMetrics{})
}

type testTracer struct{}

func (testTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, testSpan{}
}

type testSpan struct{}

func (testSpan) End()                                 {}
func (testSpan) SpanContext() application.SpanContext { return testSpanContext{} }
func (testSpan) RecordError(error)                    {}

type testSpanContext struct{}

func (testSpanContext) TraceID() string { return "trace-test" }

type testLogger struct{}

func (testLogger) DebugJSON(string, ...any)               {}
func (testLogger) InfoJSON(string, ...any)                {}
func (testLogger) WarnJSON(string, ...any)                {}
func (testLogger) ErrorJSON(string, ...any)               {}
func (testLogger) CriticalJSON(string, ...any)            {}
func (testLogger) DebugText(string, ...any)               {}
func (testLogger) InfoText(string, ...any)                {}
func (testLogger) WarnText(string, ...any)                {}
func (testLogger) ErrorText(string, ...any)               {}
func (testLogger) CriticalText(string, ...any)            {}
func (testLogger) WithTrace(context.Context) *slog.Logger { return slog.Default() }
func (testLogger) SlogJSON() *slog.Logger                 { return slog.Default() }
func (testLogger) SlogText() *slog.Logger                 { return slog.Default() }

type testMetrics struct{}

func (testMetrics) RecordRequestTotal(string, string, int)                {}
func (testMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (testMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (testMetrics) RecordTransactionTotal(string)                         {}
func (testMetrics) RecordCommandTotal(string, string)                     {}
func (testMetrics) RecordCommandDuration(string, float64)                 {}
func (testMetrics) RecordIdempotencyTotal(string)                         {}
func (testMetrics) RecordConcurrencyRetry()                               {}
func (testMetrics) RecordOutboxTotal(string, string)                      {}
