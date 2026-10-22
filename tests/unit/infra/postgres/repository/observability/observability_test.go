//go:build unit

package observability_test

import (
	"context"
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	domainhold "github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/observability"
	"github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type metricsRecorder struct{ calls int }

func (m *metricsRecorder) RecordDBQueryDuration(string, string, string, float64) { m.calls++ }
func (*metricsRecorder) RecordRequestTotal(string, string, int)                  {}
func (*metricsRecorder) RecordRequestDuration(string, string, int, float64)      {}
func (*metricsRecorder) RecordTransactionTotal(string)                           {}
func (*metricsRecorder) RecordCommandTotal(string, string)                       {}
func (*metricsRecorder) RecordCommandDuration(string, float64)                   {}
func (*metricsRecorder) RecordIdempotencyTotal(string)                           {}
func (*metricsRecorder) RecordConcurrencyRetry()                                 {}
func (*metricsRecorder) RecordOutboxTotal(string, string)                        {}

type accountRepoFake struct{ err error }

func (f accountRepoFake) Save(context.Context, *account.Account) error { return f.err }
func (f accountRepoFake) FindAccount(context.Context, criteria.AccountCriteria) (*account.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &account.Account{}, nil
}

type balanceRepoFake struct{ err error }

func (f balanceRepoFake) Save(context.Context, *balance.Balance) error { return f.err }
func (f balanceRepoFake) Find(context.Context, criteria.BalanceCriteria) (*balance.Balance, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &balance.Balance{}, nil
}

type transactionRepoFake struct{ err error }

func (f transactionRepoFake) Save(context.Context, *transaction.Transaction) error { return f.err }
func (f transactionRepoFake) Find(context.Context, transaction.TransactionCriteria) (*transaction.Transaction, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &transaction.Transaction{}, nil
}
func (f transactionRepoFake) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	return 4, 500, nil
}
func (f transactionRepoFake) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return true, nil
}

type outboxRepoFake struct{ err error }

type holdRepoFake struct {
	domainhold.Repository
	err error
}

func (f holdRepoFake) Save(context.Context, *domainhold.Instruction) error { return f.err }

func (f outboxRepoFake) Save(context.Context, *outbox.Outbox) error { return f.err }
func (f outboxRepoFake) ClaimPending(context.Context, int, int, time.Duration) ([]*outbox.Outbox, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []*outbox.Outbox{}, nil
}
func (f outboxRepoFake) FindAll(context.Context, outbox.StatusOutbox, int) ([]*outbox.Outbox, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []*outbox.Outbox{}, nil
}
func (f outboxRepoFake) UpdateOutboxData(context.Context, entity.ID, outbox.UpdateOutboxData) error {
	return f.err
}

var _ account.Repository = accountRepoFake{}
var _ balance.Repository = balanceRepoFake{}
var _ transaction.Repository = transactionRepoFake{}
var _ outbox.Repository = outboxRepoFake{}
var _ application.Metrics = (*metricsRecorder)(nil)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: OBSERVABILITY", func() {
	Describe("#ObservabilityAccountRepo", func() {
		Context("success cases", func() {
			It("should save an account and record query duration", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityAccountRepo(accountRepoFake{}, metrics, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &account.Account{})).To(Succeed())
				Expect(metrics.calls).To(Equal(1))
			})
			It("should return the found account", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityAccountRepo(accountRepoFake{}, metrics, adapter.SilentTracerMock{})
				found, err := sut.FindAccount(context.Background(), criteria.AccountCriteria{})
				Expect(err).NotTo(HaveOccurred())
				Expect(found).NotTo(BeNil())
				Expect(metrics.calls).To(Equal(1))
			})
		})
		Context("error cases", func() {
			It("should return account save errors", func() {
				wantErr := errors.New("account save failed")
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityAccountRepo(accountRepoFake{err: wantErr}, metrics, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &account.Account{})).To(MatchError(wantErr))
				Expect(metrics.calls).To(Equal(1))
			})
			It("should return account query errors", func() {
				wantErr := errors.New("account find failed")
				sut := observability.NewObservabilityAccountRepo(accountRepoFake{err: wantErr}, &metricsRecorder{}, adapter.SilentTracerMock{})
				found, err := sut.FindAccount(context.Background(), criteria.AccountCriteria{})
				Expect(found).To(BeNil())
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#ObservabilityBalanceRepo", func() {
		Context("success cases", func() {
			It("should save a balance", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityBalanceRepo(balanceRepoFake{}, metrics, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &balance.Balance{})).To(Succeed())
				Expect(metrics.calls).To(Equal(1))
			})
			It("should return the found balance", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityBalanceRepo(balanceRepoFake{}, metrics, adapter.SilentTracerMock{})
				found, err := sut.Find(context.Background(), criteria.BalanceCriteria{})
				Expect(err).NotTo(HaveOccurred())
				Expect(found).NotTo(BeNil())
				Expect(metrics.calls).To(Equal(1))
			})
		})
		Context("error cases", func() {
			It("should return balance save errors", func() {
				wantErr := errors.New("balance save failed")
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityBalanceRepo(balanceRepoFake{err: wantErr}, metrics, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &balance.Balance{})).To(MatchError(wantErr))
				Expect(metrics.calls).To(Equal(1))
			})
			It("should return balance query errors", func() {
				wantErr := errors.New("balance find failed")
				sut := observability.NewObservabilityBalanceRepo(balanceRepoFake{err: wantErr}, &metricsRecorder{}, adapter.SilentTracerMock{})
				found, err := sut.Find(context.Background(), criteria.BalanceCriteria{})
				Expect(found).To(BeNil())
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#ObservabilityTransactionRepo", func() {
		Context("success cases", func() {
			It("should save and find transactions", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityTransactionRepo(transactionRepoFake{}, metrics, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &transaction.Transaction{})).To(Succeed())
				found, err := sut.Find(context.Background(), transaction.TransactionCriteria{})
				Expect(err).NotTo(HaveOccurred())
				Expect(found).NotTo(BeNil())
				Expect(metrics.calls).To(Equal(2))
			})
			It("should return ledger state and idempotency existence", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityTransactionRepo(transactionRepoFake{}, metrics, adapter.SilentTracerMock{})
				sequence, balance, err := sut.FindLatestLedgerState(context.Background(), "account")
				Expect(err).NotTo(HaveOccurred())
				Expect(sequence).To(Equal(int64(4)))
				Expect(balance).To(Equal(int64(500)))
				exists, err := sut.ExistsByIdempotencyKey(context.Background(), "key")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(metrics.calls).To(Equal(2))
			})
		})
		Context("error cases", func() {
			It("should return transaction repository failures", func() {
				wantErr := errors.New("transaction query failed")
				sut := observability.NewObservabilityTransactionRepo(transactionRepoFake{err: wantErr}, &metricsRecorder{}, adapter.SilentTracerMock{})
				found, err := sut.Find(context.Background(), transaction.TransactionCriteria{})
				Expect(found).To(BeNil())
				Expect(err).To(MatchError(wantErr))
				sequence, balance, err := sut.FindLatestLedgerState(context.Background(), "account")
				Expect(sequence).To(BeZero())
				Expect(balance).To(BeZero())
				Expect(err).To(MatchError(wantErr))
				exists, err := sut.ExistsByIdempotencyKey(context.Background(), "key")
				Expect(exists).To(BeFalse())
				Expect(err).To(MatchError(wantErr))
			})
		})
	})

	Describe("#ObservabilityOutboxRepo", func() {
		Context("success cases", func() {
			It("should save, claim, find, and update outbox records", func() {
				metrics := &metricsRecorder{}
				sut := observability.NewObservabilityOutboxRepo(outboxRepoFake{}, metrics, adapter.SilentTracerMock{})
				ctx := context.Background()
				Expect(sut.Save(ctx, &outbox.Outbox{})).To(Succeed())
				claimed, err := sut.ClaimPending(ctx, 5, 3, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				Expect(claimed).To(BeEmpty())
				items, err := sut.FindAll(ctx, outbox.Pending, 10)
				Expect(err).NotTo(HaveOccurred())
				Expect(items).To(BeEmpty())
				id, err := entity.NewID("019ff448-c43d-70d3-83c7-dfa067446b01")
				Expect(err).NotTo(HaveOccurred())
				Expect(sut.UpdateOutboxData(ctx, id, outbox.UpdateOutboxData{})).To(Succeed())
				Expect(metrics.calls).To(Equal(4))
			})
		})
		Context("error cases", func() {
			It("should return outbox repository failures", func() {
				wantErr := errors.New("outbox operation failed")
				sut := observability.NewObservabilityOutboxRepo(outboxRepoFake{err: wantErr}, &metricsRecorder{}, adapter.SilentTracerMock{})
				Expect(sut.Save(context.Background(), &outbox.Outbox{})).To(MatchError(wantErr))
				claimed, err := sut.ClaimPending(context.Background(), 1, 1, time.Second)
				Expect(claimed).To(BeNil())
				Expect(err).To(MatchError(wantErr))
				items, err := sut.FindAll(context.Background(), outbox.Pending, 1)
				Expect(items).To(BeNil())
				Expect(err).To(MatchError(wantErr))
				id, err := entity.NewID("019ff448-c43d-70d3-83c7-dfa067446b01")
				Expect(err).NotTo(HaveOccurred())
				Expect(sut.UpdateOutboxData(context.Background(), id, outbox.UpdateOutboxData{})).To(MatchError(wantErr))
			})
		})
	})
	Describe("HoldRepository.Save", func() {
		It("should record query duration and return repository failures", func() {
			wantErr := errors.New("hold save failed")
			metrics := &metricsRecorder{}
			sut := observability.NewObservabilityHoldRepo(holdRepoFake{err: wantErr}, metrics, adapter.SilentTracerMock{})

			err := sut.Save(context.Background(), nil)

			Expect(err).To(MatchError(wantErr))
			Expect(metrics.calls).To(Equal(1))
		})
	})
})
