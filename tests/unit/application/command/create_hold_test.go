//go:build unit
// +build unit

package command_test

import (
	"context"
	"errors"

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
	existing                                                      *hold.Instruction
	saved                                                         *hold.Instruction
	byID                                                          *hold.Instruction
	release                                                       *hold.Release
	savedRelease                                                  *hold.Release
	history                                                       []hold.History
	identityErr, saveErr, historyErr, findReleaseErr, findByIDErr error
	effectiveErr, updateErr, saveReleaseErr                       error
	effective                                                     []hold.Instruction
}

func (r *holdRepositoryFake) FindByIdentity(context.Context, string, string) (*hold.Instruction, error) {
	return r.existing, r.identityErr
}

func (r *holdRepositoryFake) Save(_ context.Context, instruction *hold.Instruction) error {
	r.saved = instruction
	return r.saveErr
}
func (r *holdRepositoryFake) SaveHistory(_ context.Context, history hold.History) error {
	if r.historyErr != nil {
		return r.historyErr
	}
	r.history = append(r.history, history)
	return nil
}
func (r *holdRepositoryFake) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return r.effective, r.effectiveErr
}
func (r *holdRepositoryFake) FindRelease(context.Context, string) (*hold.Release, error) {
	return r.release, r.findReleaseErr
}
func (r *holdRepositoryFake) FindByID(context.Context, string) (*hold.Instruction, error) {
	return r.byID, r.findByIDErr
}
func (r *holdRepositoryFake) Update(_ context.Context, instruction *hold.Instruction) error {
	r.byID = instruction
	return r.updateErr
}
func (r *holdRepositoryFake) SaveRelease(_ context.Context, release hold.Release) error {
	r.savedRelease = &release
	return r.saveReleaseErr
}

type holdAccountRepoFake struct {
	account.Repository
	value *account.Account
	err   error
}

func (r holdAccountRepoFake) FindAccount(context.Context, criteria.AccountCriteria) (*account.Account, error) {
	return r.value, r.err
}

type holdTransactionRepoFake struct {
	transaction.Repository
	balance int64
	err     error
}

func (r holdTransactionRepoFake) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	return 0, r.balance, r.err
}

type holdTransactionRepoSpy struct {
	holdTransactionRepoFake
	calls int
}

func (r *holdTransactionRepoSpy) FindLatestLedgerState(ctx context.Context, accountID string) (int64, int64, error) {
	r.calls++
	return r.holdTransactionRepoFake.FindLatestLedgerState(ctx, accountID)
}

type holdOutboxRepoFake struct {
	outbox.Repository
	err error
}

func (r holdOutboxRepoFake) Save(context.Context, *outbox.Outbox) error { return r.err }

type releaseOutboxFake struct {
	holdOutboxRepoFake
	saved *outbox.Outbox
}

func (r *releaseOutboxFake) Save(_ context.Context, item *outbox.Outbox) error {
	r.saved = item
	return r.holdOutboxRepoFake.err
}

type holdPolicyFake struct {
	found  bool
	policy hold.AllocationPolicy
	err    error
}

func (r holdPolicyFake) PolicyFor(context.Context, account.Account, hold.HoldType) (hold.AllocationPolicy, bool, error) {
	return r.policy, r.found, r.err
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
			It("should allocate an eligible request when the configured policy and balance allow it", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{}
				policy := hold.AllocationPolicy{Version: "policy-v1", TypeOrder: []hold.HoldType{hold.Judicial}, UnresolvedConflict: hold.PendingReviewOnConflict}
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}}, holdTransactionRepoFake{balance: 1000}, repo, holdPolicyFake{found: true, policy: policy}, holdOutboxRepoFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "funded", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusAllocated))
				Expect(result.Instruction.Allocated).To(Equal(requested))
				Expect(result.Instruction.PolicyVersion).To(Equal("policy-v1"))
			})
			It("should leave an eligible request pending when the balance is insufficient and partial allocation is disabled", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{}
				policy := hold.AllocationPolicy{Version: "policy-v1", TypeOrder: []hold.HoldType{hold.Judicial}, UnresolvedConflict: hold.PendingReviewOnConflict}
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}}, holdTransactionRepoFake{balance: 100}, repo, holdPolicyFake{found: true, policy: policy}, holdOutboxRepoFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "insufficient", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusPendingReview))
				Expect(result.Instruction.Allocated.Amount()).To(BeZero())
				Expect(result.Instruction.Pending).To(Equal(requested))
				Expect(repo.history[0].Reason).To(Equal("ELIGIBLE_FUNDS_INSUFFICIENT"))
			})
			It("should partially allocate only funds not already retained by another hold", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				allocated, _ := money.NewMoney(100, money.BRL)
				released, _ := money.NewMoney(60, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{effective: []hold.Instruction{{ID: "existing", Type: hold.Regulatory, Allocated: allocated, Released: released}}}
				policy := hold.AllocationPolicy{Version: "policy-v1", TypeOrder: []hold.HoldType{hold.Judicial}, AllowPartial: true, UnresolvedConflict: hold.PendingReviewOnConflict}
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}}, holdTransactionRepoFake{balance: 200}, repo, holdPolicyFake{found: true, policy: policy}, holdOutboxRepoFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "partial", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusPartiallyAllocated))
				Expect(result.Instruction.Allocated.Amount()).To(Equal(int64(160)))
				Expect(result.Instruction.Pending.Amount()).To(Equal(int64(340)))
				Expect(repo.saved.AllowPartial).To(BeTrue())
			})
			It("should persist an ineligible account request for review without reading its balance", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{}
				transactions := &holdTransactionRepoSpy{}
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: &account.Account{ID: accountID, Currency: money.BRL}}, transactions, repo, nil, holdOutboxRepoFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "ineligible", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusPendingReview))
				Expect(repo.history[0].Reason).To(Equal("HOLD_NOT_ELIGIBLE"))
				Expect(transactions.calls).To(BeZero())
			})
		})
		Context("error cases", func() {
			It("should propagate an error while checking the instruction identity", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				cause := errors.New("identity lookup failed")
				repo := &holdRepositoryFake{identityErr: cause}
				sut := newCreateHoldCommand(repo, nil, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "origin", Reference: "identity-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should return account not found without persisting an instruction", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				repo := &holdRepositoryFake{}
				sut := newCreateHoldCommand(repo, nil, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: "missing", Currency: string(money.BRL), Origin: "origin", Reference: "missing-account", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring(account.ErrAccountNotFound.Error())))
				Expect(repo.saved).To(BeNil())
			})
			It("should return a currency mismatch when the hold currency differs from the account", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				repo := &holdRepositoryFake{}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.USD}, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "currency", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(money.ErrCurrencyMismatch))
			})
			It("should return a policy provider error without persisting a hold", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("policy service unavailable")
				repo := &holdRepositoryFake{}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}, holdTransactionRepoFake{}, holdPolicyFake{err: cause}, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "policy-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
				Expect(repo.saved).To(BeNil())
			})
			It("should propagate ledger state lookup errors", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("ledger unavailable")
				repo := &holdRepositoryFake{}
				policy := hold.AllocationPolicy{Version: "policy-v1", TypeOrder: []hold.HoldType{hold.Judicial}, UnresolvedConflict: hold.PendingReviewOnConflict}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}, holdTransactionRepoFake{err: cause}, holdPolicyFake{found: true, policy: policy}, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "ledger-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate effective hold lookup errors", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("effective holds unavailable")
				repo := &holdRepositoryFake{effectiveErr: cause}
				policy := hold.AllocationPolicy{Version: "policy-v1", TypeOrder: []hold.HoldType{hold.Judicial}, UnresolvedConflict: hold.PendingReviewOnConflict}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL, HoldsEligible: true}, holdTransactionRepoFake{balance: 1000}, holdPolicyFake{found: true, policy: policy}, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "active-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate an instruction insert error", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("hold insert failed")
				repo := &holdRepositoryFake{saveErr: cause}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL}, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "insert-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should return a history persistence error after saving the instruction", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("history write failed")
				repo := &holdRepositoryFake{historyErr: cause}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL}, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "history-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
				Expect(repo.saved).NotTo(BeNil())
				Expect(repo.history).To(BeEmpty())
			})
			It("should return an outbox persistence error after recording the instruction and history", func() {
				requested, _ := money.NewMoney(500, money.BRL)
				accountID, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())
				cause := errors.New("outbox write failed")
				repo := &holdRepositoryFake{}
				sut := newCreateHoldCommand(repo, &account.Account{ID: accountID, Currency: money.BRL}, holdTransactionRepoFake{}, nil, holdOutboxRepoFake{err: cause})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "outbox-error", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
				Expect(repo.saved).NotTo(BeNil())
				Expect(repo.history).To(HaveLen(1))
			})
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

func newCreateHoldCommand(repo *holdRepositoryFake, accountEntity *account.Account, transactions transaction.Repository, policy command.HoldPolicyProvider, outboxRepo outbox.Repository) *command.CreateHold {
	return command.NewCreateHold(
		holdUOW{}, holdAccountRepoFake{value: accountEntity}, transactions,
		repo, policy, outboxRepo,
		adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{},
	)
}

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
			It("should reject invalid release input before opening a transaction", func() {
				sut := command.NewReleaseHold(holdUOW{}, &holdRepositoryFake{}, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{})

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrInvalidRelease))
			})
			It("should reject a release when its hold cannot be found", func() {
				amount, _ := money.NewMoney(100, money.BRL)
				sut := command.NewReleaseHold(holdUOW{}, &holdRepositoryFake{}, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "missing", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring(hold.ErrInvalidRelease.Error())))
			})
			It("should preserve repository errors when recording a release", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				cause := errors.New("release insert failed")
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction, saveReleaseErr: cause}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should reject a repeated release key with a different request fingerprint", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{holdOutboxRepoFake: holdOutboxRepoFake{err: errors.New("outbox failed")}}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount}

				_, err := sut.Execute(context.Background(), input)
				Expect(err).To(HaveOccurred())
				// A different payload under the same key is rejected after the failed attempt.
				repo.release = &hold.Release{RequestFingerprint: "different"}
				input.AuthorizationReference = "other-auth"
				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrReleaseConflict))
			})
			It("should propagate a release lookup error", func() {
				amount, _ := money.NewMoney(100, money.BRL)
				cause := errors.New("release lookup failed")
				repo := &holdRepositoryFake{findReleaseErr: cause}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate a hold lookup error", func() {
				amount, _ := money.NewMoney(100, money.BRL)
				cause := errors.New("hold lookup failed")
				repo := &holdRepositoryFake{findByIDErr: cause}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate an instruction update error", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				cause := errors.New("hold update failed")
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction, updateErr: cause}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate a release history write error", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				cause := errors.New("history write failed")
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction, historyErr: cause}
				sut := command.NewReleaseHold(holdUOW{}, repo, &releaseOutboxFake{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should propagate an outbox write error after recording the release", func() {
				allocated, _ := money.NewMoney(1000, money.BRL)
				zero, _ := money.NewMoney(0, money.BRL)
				amount, _ := money.NewMoney(400, money.BRL)
				cause := errors.New("outbox write failed")
				instruction := &hold.Instruction{ID: "hold-1", Requested: allocated, Allocated: allocated, Pending: zero, Released: zero, Status: hold.StatusAllocated}
				repo := &holdRepositoryFake{byID: instruction}
				box := &releaseOutboxFake{holdOutboxRepoFake: holdOutboxRepoFake{err: cause}}
				sut := command.NewReleaseHold(holdUOW{}, repo, box, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				result, err := sut.Execute(context.Background(), dto.ReleaseHoldInput{HoldID: "hold-1", IdempotencyKey: "release", AuthorizationReference: "auth", Amount: amount})

				Expect(result).To(BeNil())
				Expect(errors.Is(err, cause)).To(BeTrue())
				Expect(box.saved).NotTo(BeNil())
			})
		})
	})
})
