//go:build unit
// +build unit

package command_test

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type holdUOW struct{ application.UnitOfWork }

func (u holdUOW) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type holdRepositoryFake struct {
	hold.Repository
	existing     *hold.Instruction
	saved        *hold.Instruction
	byID         *hold.Instruction
	release      *hold.Release
	savedRelease *hold.Release
	history      []hold.History
}

func (r *holdRepositoryFake) FindByIdentity(context.Context, string, string) (*hold.Instruction, error) {
	return r.existing, nil
}

func (r *holdRepositoryFake) Save(_ context.Context, instruction *hold.Instruction) error {
	r.saved = instruction
	return nil
}
func (r *holdRepositoryFake) SaveHistory(_ context.Context, history hold.History) error {
	r.history = append(r.history, history)
	return nil
}
func (*holdRepositoryFake) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return nil, nil
}
func (r *holdRepositoryFake) FindRelease(context.Context, string) (*hold.Release, error) {
	return r.release, nil
}
func (r *holdRepositoryFake) FindByID(context.Context, string) (*hold.Instruction, error) {
	return r.byID, nil
}
func (r *holdRepositoryFake) Update(_ context.Context, instruction *hold.Instruction) error {
	r.byID = instruction
	return nil
}
func (r *holdRepositoryFake) SaveRelease(_ context.Context, release hold.Release) error {
	r.savedRelease = &release
	return nil
}

type holdAccountRepoFake struct {
	account.Repository
	value *account.Account
}

func (r holdAccountRepoFake) FindAccount(context.Context, criteria.AccountCriteria) (*account.Account, error) {
	return r.value, nil
}

type holdTransactionRepoFake struct {
	transaction.Repository
	balance int64
}

func (r holdTransactionRepoFake) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	return 0, r.balance, nil
}

type holdOutboxRepoFake struct{ outbox.Repository }

func (holdOutboxRepoFake) Save(context.Context, *outbox.Outbox) error { return nil }

type releaseOutboxFake struct {
	holdOutboxRepoFake
	saved *outbox.Outbox
}

func (r *releaseOutboxFake) Save(_ context.Context, item *outbox.Outbox) error {
	r.saved = item
	return nil
}

type holdPolicyFake struct {
	found  bool
	policy hold.AllocationPolicy
}

func (r holdPolicyFake) PolicyFor(context.Context, account.Account, hold.HoldType) (hold.AllocationPolicy, bool, error) {
	return r.policy, r.found, nil
}

type recordingHoldMetrics struct {
	adaptermocks.SilentMetricsMock
	commandName, commandState string
	idempotencyResults        []string
	durationCalls             int
}

func (m *recordingHoldMetrics) RecordCommandTotal(commandName, state string) {
	m.commandName, m.commandState = commandName, state
}

func (m *recordingHoldMetrics) RecordCommandDuration(string, float64) { m.durationCalls++ }

func (m *recordingHoldMetrics) RecordIdempotencyTotal(result string) {
	m.idempotencyResults = append(m.idempotencyResults, result)
}

type recordingHoldTracer struct {
	adaptermocks.SilentTracerMock
	spanName string
	span     *recordingHoldSpan
}

func (t *recordingHoldTracer) Start(ctx context.Context, name string) (context.Context, application.Span) {
	_, baseSpan := adaptermocks.SilentTracerMock{}.Start(ctx, name)
	t.spanName = name
	t.span = &recordingHoldSpan{Span: baseSpan}
	return ctx, t.span
}

type recordingHoldSpan struct {
	application.Span
	ended       bool
	recordedErr error
}

func (s *recordingHoldSpan) End() { s.ended = true }

func (s *recordingHoldSpan) RecordError(err error) { s.recordedErr = err }

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE HOLD", func() {
	Describe("#Execute", func() {
		Context("success cases", func() {
			It("should return the stored instruction for an identical replay", func() {
				requested, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "origin", Reference: "reference", Type: hold.Judicial, Requested: requested}
				stored, err := input.CreateHoldFacade()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{existing: stored}
				sut := command.NewCreateHold(holdUOW{}, nil, nil, repo, nil, nil, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.IdempotentReplay).To(BeTrue())
				Expect(result.Instruction).To(BeIdenticalTo(stored))
			})
			It("should persist an eligible instruction as pending when no policy is configured", func() {
				requested, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				accountEntity := &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}
				repo := &holdRepositoryFake{}
				tracer := &recordingHoldTracer{}
				metrics := &recordingHoldMetrics{}
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: accountEntity}, holdTransactionRepoFake{}, repo, holdPolicyFake{}, holdOutboxRepoFake{}, tracer, adaptermocks.SilentLoggerMock{}, metrics)
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "unconfigured-policy", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusPendingReview))
				Expect(result.Instruction.Allocated.Amount()).To(BeZero())
				Expect(result.Instruction.Pending).To(Equal(requested))
				Expect(repo.saved).To(BeIdenticalTo(result.Instruction))
				Expect(tracer.spanName).To(Equal("CreateHold.Execute"))
				Expect(tracer.span.ended).To(BeTrue())
				Expect(metrics.commandName).To(Equal("CreateHold"))
				Expect(metrics.commandState).To(Equal("pending"))
				Expect(metrics.idempotencyResults).To(Equal([]string{"new"}))
				Expect(metrics.durationCalls).To(Equal(1))
			})
		})
		Context("error cases", func() {
			It("should reject an identity replay with a different fingerprint", func() {
				requested, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "origin", Reference: "reference", Type: hold.Judicial, Requested: requested}
				stored, err := input.CreateHoldFacade()
				Expect(err).NotTo(HaveOccurred())
				stored.RequestFingerprint = "different-fingerprint"
				sut := command.NewCreateHold(holdUOW{}, nil, nil, &holdRepositoryFake{existing: stored}, nil, nil, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrHoldConflict))
			})
		})
	})
})

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RELEASE HOLD", func() {
	Describe("#Execute", func() {
		Context("success cases", func() {
			It("should release a partial amount and persist its audit event", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				instruction := &hold.Instruction{ID: "hold-1", AccountID: "account-1", Currency: "BRL", Origin: "system", Reference: "ref", Type: hold.Judicial, Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction}
				box := &releaseOutboxFake{}
				tracer := &recordingHoldTracer{}
				metrics := &recordingHoldMetrics{}
				sut := command.NewReleaseHold(holdUOW{}, repo, box, tracer, adaptermocks.SilentLoggerMock{}, metrics)

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release-1", AuthorizationReference: "auth-1", Amount: amount})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Released).To(Equal(amount))
				Expect(result.Instruction.Status).To(Equal(hold.StatusAllocated))
				Expect(repo.savedRelease.RequestFingerprint).NotTo(BeEmpty())
				Expect(repo.history).To(HaveLen(1))
				Expect(repo.history[0].EventType).To(Equal("RELEASE_RECORDED"))
				Expect(box.saved.EventType).To(Equal(outbox.HoldReleased))
				Expect(tracer.spanName).To(Equal("ReleaseHold.Execute"))
				Expect(tracer.span.ended).To(BeTrue())
				Expect(metrics.commandName).To(Equal("ReleaseHold"))
				Expect(metrics.commandState).To(Equal("success"))
				Expect(metrics.idempotencyResults).To(Equal([]string{"new"}))
				Expect(metrics.durationCalls).To(Equal(1))
			})
			It("should mark the hold released when the remaining retained amount is released", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				alreadyReleased, _ := money.NewMoney(250, money.BRL)
				pending, _ := money.NewMoney(0, money.BRL)
				remaining, _ := money.NewMoney(750, money.BRL)
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: pending, Released: alreadyReleased, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release-final", AuthorizationReference: "auth-final", Amount: remaining})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Released.Amount()).To(Equal(int64(1000)))
				Expect(result.Instruction.Status).To(Equal(hold.StatusReleased))
			})
			It("should return the existing release for an identical replay", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				input := dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release-1", AuthorizationReference: "auth-1", Amount: amount}
				instruction := &hold.Instruction{ID: "hold-1", AccountID: "account-1", Currency: "BRL", Origin: "system", Reference: "ref", Type: hold.Judicial, Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err := sut.Execute(context.Background(), input)
				Expect(err).NotTo(HaveOccurred())
				stored := repo.savedRelease
				repo.release = stored

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.IdempotentReplay).To(BeTrue())
				Expect(result.Release).To(BeIdenticalTo(stored))
			})
		})
		Context("error cases", func() {
			It("should reject reuse of a release key with different content", func() {
				amount, _ := money.NewMoney(400, money.BRL)
				input := dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release-1", AuthorizationReference: "auth-1", Amount: amount}
				repo := &holdRepositoryFake{release: &hold.Release{RequestFingerprint: "different"}}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrReleaseConflict))
			})
			It("should reject a release that exceeds the retained amount", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(1001, money.BRL)
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				sut := command.NewReleaseHold(holdUOW{}, &holdRepositoryFake{byID: instruction}, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release-1", AuthorizationReference: "auth-1", Amount: amount})

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrInvalidRelease))
				Expect(instruction.Released.Amount()).To(BeZero())
			})
		})
	})
})
