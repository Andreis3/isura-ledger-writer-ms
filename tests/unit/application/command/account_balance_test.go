//go:build unit

package command_test

import (
	"context"
	"errors"
	"log/slog"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("account and balance commands", func() {
	It("persists and publishes an account", func() {
		repo := &accountRepositoryFake{}
		publisher := &publisherFake{}
		sut := command.NewCreateAccount(repo, publisher, silentLogger{}, silentTracer{}, silentMetrics{})
		input := validAccountInput()

		result, err := sut.Execute(context.Background(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.AccountID).NotTo(BeNil())
		Expect(repo.saved).NotTo(BeNil())
		Expect(publisher.events).To(HaveLen(1))
	})

	It("returns an existing account", func() {
		existing := validAccount()
		repo := &accountRepositoryFake{found: existing}
		sut := command.NewCreateAccount(repo, &publisherFake{}, silentLogger{}, silentTracer{}, silentMetrics{})

		result, err := sut.Execute(context.Background(), validAccountInput())

		Expect(err).NotTo(HaveOccurred())
		Expect(*result.AccountID).To(Equal(existing.ID.String()))
		Expect(repo.saved).To(BeNil())
	})

	It("returns validation and publish errors", func() {
		sut := command.NewCreateAccount(&accountRepositoryFake{}, &publisherFake{}, silentLogger{}, silentTracer{}, silentMetrics{})
		_, err := sut.Execute(context.Background(), dto.CreateAccountInput{})
		Expect(err).To(HaveOccurred())

		repo := &accountRepositoryFake{}
		publisher := &publisherFake{err: errors.New("publisher unavailable")}
		sut = command.NewCreateAccount(repo, publisher, silentLogger{}, silentTracer{}, silentMetrics{})
		_, err = sut.Execute(context.Background(), validAccountInput())
		Expect(err).To(HaveOccurred())
	})

	It("persists a balance for an existing account", func() {
		accountEntity := validAccount()
		repo := &balanceRepositoryFake{}
		sut := command.NewCreateBalance(repo, &accountRepositoryFake{found: accountEntity}, silentLogger{}, silentTracer{}, silentMetrics{})

		err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: accountEntity.ID.String(), Currency: "BRL"})

		Expect(err).NotTo(HaveOccurred())
		Expect(repo.saved).NotTo(BeNil())
	})

	It("handles existing and missing accounts when creating a balance", func() {
		accountEntity := validAccount()
		existingBalance := validBalance(accountEntity.ID.String())
		repo := &balanceRepositoryFake{found: existingBalance}
		sut := command.NewCreateBalance(repo, &accountRepositoryFake{found: accountEntity}, silentLogger{}, silentTracer{}, silentMetrics{})

		Expect(sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: accountEntity.ID.String(), Currency: "BRL"})).NotTo(HaveOccurred())

		repo = &balanceRepositoryFake{}
		sut = command.NewCreateBalance(repo, &accountRepositoryFake{}, silentLogger{}, silentTracer{}, silentMetrics{})
		err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: accountEntity.ID.String(), Currency: "BRL"})
		Expect(err).To(HaveOccurred())
	})
})

func validAccountInput() dto.CreateAccountInput {
	return dto.CreateAccountInput{
		AccountExternalID: uuid.NewString(),
		AccountNumber:     "123456",
		TaxID:             "529.982.247-25",
		AccountType:       "ASSET",
		BalancePolicy:     "BALANCE_NON_NEGATIVE",
		Currency:          "BRL",
	}
}

func validAccount() *account.Account {
	result, err := account.NewAccountBuilder().WithID().WithAccountExternalID(uuid.NewString()).WithAccountNumber("123456").WithTaxID("529.982.247-25").WithStatus().WithType("ASSET").WithBalancePolicy("BALANCE_NON_NEGATIVE").WithCurrency("BRL").Build()
	if err != nil {
		panic(err)
	}
	return result
}

func validBalance(accountID string) *balance.Balance {
	result, err := balance.NewBalanceBuilder().WithID().WithAccountID(accountID).WithAmount(0, "BRL").Build()
	if err != nil {
		panic(err)
	}
	return result
}

type accountRepositoryFake struct {
	found *account.Account
	saved *account.Account
}

func (f *accountRepositoryFake) Save(_ context.Context, value *account.Account) error {
	f.saved = value
	return nil
}
func (f *accountRepositoryFake) FindAccount(_ context.Context, _ criteria.AccountCriteria) (*account.Account, error) {
	if f.found == nil {
		return nil, nil
	}
	return f.found, nil
}

type balanceRepositoryFake struct {
	found *balance.Balance
	saved *balance.Balance
}

func (f *balanceRepositoryFake) Save(_ context.Context, value *balance.Balance) error {
	f.saved = value
	return nil
}
func (f *balanceRepositoryFake) Find(_ context.Context, _ criteria.BalanceCriteria) (*balance.Balance, error) {
	if f.found == nil {
		return nil, balance.ErrBalanceNotFound
	}
	return f.found, nil
}

type publisherFake struct {
	events []event.Event
	err    error
}

func (f *publisherFake) Publish(_ context.Context, value event.Event) error {
	f.events = append(f.events, value)
	return f.err
}

type silentLogger struct{}

func (silentLogger) DebugJSON(string, ...any)               {}
func (silentLogger) InfoJSON(string, ...any)                {}
func (silentLogger) WarnJSON(string, ...any)                {}
func (silentLogger) ErrorJSON(string, ...any)               {}
func (silentLogger) CriticalJSON(string, ...any)            {}
func (silentLogger) DebugText(string, ...any)               {}
func (silentLogger) InfoText(string, ...any)                {}
func (silentLogger) WarnText(string, ...any)                {}
func (silentLogger) ErrorText(string, ...any)               {}
func (silentLogger) CriticalText(string, ...any)            {}
func (silentLogger) WithTrace(context.Context) *slog.Logger { return slog.Default() }
func (silentLogger) SlogJSON() *slog.Logger                 { return slog.Default() }
func (silentLogger) SlogText() *slog.Logger                 { return slog.Default() }

type silentTracer struct{}

func (silentTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, silentSpan{}
}

type silentSpan struct{}

func (silentSpan) End()                                 {}
func (silentSpan) RecordError(error)                    {}
func (silentSpan) SpanContext() application.SpanContext { return silentSpanContext{} }

type silentSpanContext struct{}

func (silentSpanContext) TraceID() string { return "test-trace" }

type silentMetrics struct{}

func (silentMetrics) RecordRequestTotal(string, string, int)                {}
func (silentMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (silentMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (silentMetrics) RecordTransactionTotal(string)                         {}
func (silentMetrics) RecordCommandTotal(string, string)                     {}
func (silentMetrics) RecordCommandDuration(string, float64)                 {}
func (silentMetrics) RecordIdempotencyTotal(string)                         {}
func (silentMetrics) RecordConcurrencyRetry()                               {}
func (silentMetrics) RecordOutboxTotal(string, string)                      {}
