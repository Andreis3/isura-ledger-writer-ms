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
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
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
		metrics := newTestMetrics()
		sut := newCommand(accounts, transactions, outboxes, &unitOfWork{}, metrics)

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal(string(transaction.Completed)))
		Expect(result.IdempotentReplay).To(BeFalse())
		Expect(transactions.saved).NotTo(BeNil())
		Expect(transactions.saved.Status).To(Equal(transaction.Completed))
		Expect(transactions.saved.Entries).To(HaveLen(2))
		Expect(outboxes.saved).NotTo(BeNil())
		Expect(metrics.idempotencyCounts).To(Equal(map[string]int{"new": 1}))
		Expect(accounts.findCalls).To(Equal([]string{creditExternalID, debitExternalID}))

		var event transaction.TransactionCreated
		Expect(json.Unmarshal(outboxes.saved.Payload, &event)).To(Succeed())
		Expect(event.EventID).NotTo(BeEmpty())
		Expect(event.Status).To(Equal(string(transaction.Completed)))
		Expect(event.Metadata).To(Equal(input.Metadata))
		Expect(event.Entries).To(HaveLen(len(transactions.saved.Entries)))
		for position, entry := range transactions.saved.Entries {
			Expect(event.Entries[position]).To(Equal(transaction.TransactionEntryCreated{
				Position:  int64(position),
				AccountID: entry.AccountID,
				Direction: entry.Direction,
				Amount:    entry.Amount.Amount(),
				Currency:  string(entry.Amount.Currency()),
			}))
		}
	})

	It("creates an ordered multi-entry transaction with each distinct account loaded once", func() {
		input := validInput()
		input.DebitAccountID = nil
		input.CreditAccountID = nil
		input.Currency = nil
		input.Entries = []dto.EntryInput{
			{AccountID: debitExternalID, Direction: string(transaction.Debit), Amount: 150000, Currency: string(money.BRL)},
			{AccountID: creditExternalID, Direction: string(transaction.Credit), Amount: 100000, Currency: string(money.BRL)},
			{AccountID: creditExternalID, Direction: string(transaction.Credit), Amount: 50000, Currency: string(money.BRL)},
		}
		accounts := newAccountRepository()
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{}
		sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IdempotentReplay).To(BeFalse())
		Expect(transactions.saved.Entries).To(HaveLen(3))
		Expect(accounts.findCalls).To(Equal([]string{creditExternalID, debitExternalID}))
		Expect(transactions.saved.Entries[0].AccountID).To(Equal(accounts.accounts[debitExternalID].ID.String()))
		Expect(transactions.saved.Entries[1].AccountID).To(Equal(accounts.accounts[creditExternalID].ID.String()))
		Expect(transactions.saved.Entries[2].AccountID).To(Equal(accounts.accounts[creditExternalID].ID.String()))
		Expect(transactions.saved.Entries[0].TransactionPosition).To(Equal(int64(0)))
		Expect(transactions.saved.Entries[1].TransactionPosition).To(Equal(int64(1)))
		Expect(transactions.saved.Entries[2].TransactionPosition).To(Equal(int64(2)))
		Expect(transactions.saved.Entries[1].SequenceNumber).To(Equal(int64(1)))
		Expect(transactions.saved.Entries[2].SequenceNumber).To(Equal(int64(2)))
		Expect(outboxes.saved).NotTo(BeNil())
		Expect(string(outboxes.saved.Payload)).NotTo(ContainSubstring("debit_account_id"))
		Expect(string(outboxes.saved.Payload)).NotTo(ContainSubstring("credit_account_id"))
	})

	It("rejects a composition above the configured entry limit before opening the unit of work", func() {
		input := validInput()
		input.DebitAccountID = nil
		input.CreditAccountID = nil
		input.Currency = nil
		input.Entries = []dto.EntryInput{
			{AccountID: debitExternalID, Direction: string(transaction.Debit), Amount: 150000, Currency: string(money.BRL)},
			{AccountID: creditExternalID, Direction: string(transaction.Credit), Amount: 100000, Currency: string(money.BRL)},
			{AccountID: creditExternalID, Direction: string(transaction.Credit), Amount: 50000, Currency: string(money.BRL)},
		}
		accounts := newAccountRepository()
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{}
		uow := &observingUnitOfWork{}
		sut := newCommandWithLimit(accounts, transactions, outboxes, uow, 2)

		result, err := sut.Execute(context.Background(), input)

		Expect(result).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(uow.calls).To(BeZero())
		Expect(accounts.findCalls).To(BeEmpty())
		Expect(transactions.saved).To(BeNil())
		Expect(outboxes.saved).To(BeNil())
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

	It("rejects an account whose currency differs from its requested entry", func() {
		input := validInput()
		accounts := newAccountRepository()
		accounts.accounts[creditExternalID].Currency = money.USD
		transactions := &transactionRepository{}
		outboxes := &outboxRepository{}
		sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

		result, err := sut.Execute(context.Background(), input)

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
		metrics := newTestMetrics()
		sut := newCommand(accounts, transactions, &outboxRepository{}, &unitOfWork{}, metrics)

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.TransactionID).To(Equal(new(persisted.ID.String())))
		Expect(result.Status).To(Equal(string(transaction.Completed)))
		Expect(result.IdempotentReplay).To(BeTrue())
		Expect(transactions.saved).To(BeNil())
		Expect(transactions.findCriteria.IdempotencyKey).NotTo(BeNil())
		Expect(*transactions.findCriteria.IdempotencyKey).To(Equal(*input.IdempotencyKey))
		Expect(metrics.idempotencyCounts).To(Equal(map[string]int{"replay": 1}))
	})

	It("rejects reuse of an idempotency key with a different fingerprint", func() {
		input := validInput()
		persisted, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		persisted.Fingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		accounts := newAccountRepository()
		transactions := &transactionRepository{existing: persisted}
		metrics := newTestMetrics()
		sut := newCommand(accounts, transactions, &outboxRepository{}, &unitOfWork{}, metrics)

		result, err := sut.Execute(context.Background(), input)

		Expect(result).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("idempotency fingerprint mismatch")))
		Expect(transactions.saved).To(BeNil())
		Expect(metrics.idempotencyCounts).To(Equal(map[string]int{"conflict": 1}))
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
		Expect(transactions.findCalls).To(Equal(2))
		Expect(transactions.attempts).To(HaveLen(2))
		Expect(transactions.attempts[0]).NotTo(BeIdenticalTo(transactions.attempts[1]))
		Expect(transactions.attempts[0].ID).NotTo(Equal(transactions.attempts[1].ID))
		for index := range transactions.attempts[0].Entries {
			Expect(transactions.attempts[0].Entries[index].ID).NotTo(Equal(transactions.attempts[1].Entries[index].ID))
		}
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
		metrics := newTestMetrics()
		sut := newCommand(newAccountRepository(), transactions, &outboxRepository{}, &unitOfWork{}, metrics)

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.TransactionID).To(Equal(new(persisted.ID.String())))
		Expect(result.IdempotentReplay).To(BeTrue())
		Expect(metrics.idempotencyCounts).To(Equal(map[string]int{"replay": 1}))
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
		metrics := newTestMetrics()
		sut := newCommand(newAccountRepository(), transactions, &outboxRepository{}, &unitOfWork{}, metrics)

		result, err := sut.Execute(context.Background(), input)

		Expect(result).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("idempotency fingerprint mismatch")))
		Expect(transactions.saved).To(BeNil())
		Expect(metrics.idempotencyCounts).To(Equal(map[string]int{"conflict": 1}))
	})
})

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE TRANSACTION", func() {
	Describe("#Execute", func() {
		Context("error cases", func() {
			It("should reject different external IDs that resolve to the same account without persisting", func() {
				input := validInput()
				accounts := newAccountRepository()
				accounts.accounts[creditExternalID].ID = accounts.accounts[debitExternalID].ID
				transactions := &transactionRepository{}
				outboxes := &outboxRepository{}
				sut := newCommand(accounts, transactions, outboxes, &unitOfWork{})

				for range 2 {
					result, err := sut.Execute(context.Background(), input)

					Expect(result).To(BeNil())
					var domainErr *fault.DomainError
					Expect(errors.As(err, &domainErr)).To(BeTrue())
					Expect(domainErr.Code).To(Equal(fault.CodeInvalidTransfer))
				}

				Expect(transactions.findCalls).To(Equal(2))
				Expect(transactions.saveCalls).To(BeZero())
				Expect(transactions.saved).To(BeNil())
				Expect(outboxes.saved).To(BeNil())
				Expect(accounts.findCalls).To(HaveLen(4))
			})
		})
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

type observingUnitOfWork struct {
	calls int
}

func (u *observingUnitOfWork) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
	return fn(ctx)
}

func (u *observingUnitOfWork) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
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
	return &accountRepository{
		accounts: map[string]*account.Account{
			debitExternalID:  {ID: debitID, AccountExternalID: debitExternalID, Status: account.StatusActive, AccountType: account.Liability, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL},
			creditExternalID: {ID: creditID, AccountExternalID: creditExternalID, Status: account.StatusActive, AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted, Currency: money.BRL},
		},
	}
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
	findCalls      int
	saveErrors     []error
	saveCalls      int
	attempts       []*transaction.Transaction
	ledgerStates   map[string]account.LedgerState
}

func (r *transactionRepository) Save(_ context.Context, value *transaction.Transaction) error {
	r.saveCalls++
	r.attempts = append(r.attempts, value)
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
	r.findCalls++
	r.findCriteria = params
	if r.existing == nil {
		return nil, transaction.ErrTransactionNotFound
	}
	return r.existing, nil
}

func (r *transactionRepository) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	return r.existing != nil, nil
}

func (r *transactionRepository) FindLatestLedgerState(_ context.Context, accountID string) (int64, int64, error) {
	state := r.ledgerStates[accountID]
	return state.SequenceNumber, state.RunningBalance, nil
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

func newCommand(accounts *accountRepository, transactions *transactionRepository, outboxes *outboxRepository, uow application.UnitOfWork, metrics ...*testMetrics) *command.CreateTransaction {
	return newCommandWithLimit(accounts, transactions, outboxes, uow, application.DefaultMaxTransactionEntries, metrics...)
}

func newCommandWithLimit(accounts *accountRepository, transactions *transactionRepository, outboxes *outboxRepository, uow application.UnitOfWork, maxEntries int, metrics ...*testMetrics) *command.CreateTransaction {
	var recorder application.Metrics = newTestMetrics()
	if len(metrics) > 0 {
		recorder = metrics[0]
	}
	return command.NewCreateTransaction(uow, accounts, transactions, outboxes, testTracer{}, testLogger{}, recorder, maxEntries)
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

type testMetrics struct {
	idempotencyCounts map[string]int
}

func newTestMetrics() *testMetrics {
	return &testMetrics{idempotencyCounts: make(map[string]int)}
}

func (testMetrics) RecordRequestTotal(string, string, int)                {}
func (testMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (testMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (testMetrics) RecordTransactionTotal(string)                         {}
func (testMetrics) RecordCommandTotal(string, string)                     {}
func (testMetrics) RecordCommandDuration(string, float64)                 {}
func (m *testMetrics) RecordIdempotencyTotal(state string)                { m.idempotencyCounts[state]++ }
func (testMetrics) RecordConcurrencyRetry()                               {}
func (testMetrics) RecordOutboxTotal(string, string)                      {}
