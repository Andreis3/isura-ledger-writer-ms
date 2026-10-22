//go:build unit
// +build unit

package command_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/application/security"
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

type resolutionUOW struct{}

func (resolutionUOW) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (resolutionUOW) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type resolutionHoldRepo struct {
	hold.Repository
	instruction *hold.Instruction
	decision    *hold.Decision
	histories   []hold.History
	active      []hold.Instruction
	decisionErr error
	findErr     error
	saveErr     error
	updateErr   error
	historyErr  error
	activeErr   error
}

func (r *resolutionHoldRepo) FindDecision(context.Context, string) (*hold.Decision, error) {
	return r.decision, r.decisionErr
}
func (r *resolutionHoldRepo) FindByID(context.Context, string) (*hold.Instruction, error) {
	return r.instruction, r.findErr
}
func (r *resolutionHoldRepo) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return r.active, r.activeErr
}
func (r *resolutionHoldRepo) SaveDecision(_ context.Context, decision hold.Decision) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.decision = &decision
	return nil
}
func (r *resolutionHoldRepo) Update(_ context.Context, instruction *hold.Instruction) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.instruction = instruction
	return nil
}
func (r *resolutionHoldRepo) SaveHistory(_ context.Context, history hold.History) error {
	if r.historyErr != nil {
		return r.historyErr
	}
	r.histories = append(r.histories, history)
	return nil
}

type resolutionOutboxRepo struct {
	outbox.Repository
	saved   *outbox.Outbox
	err     error
	onError func()
}

func (r *resolutionOutboxRepo) Save(_ context.Context, item *outbox.Outbox) error {
	if r.err != nil {
		if r.onError != nil {
			r.onError()
		}
		return r.err
	}
	r.saved = item
	return nil
}

type resolutionAccountRepo struct {
	account.Repository
	value *account.Account
	err   error
}

func (r resolutionAccountRepo) FindAccount(context.Context, criteria.AccountCriteria) (*account.Account, error) {
	return r.value, r.err
}

type resolutionTransactionRepo struct {
	transaction.Repository
	sequence int64
	balance  int64
	err      error
}

func (r resolutionTransactionRepo) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	return r.sequence, r.balance, r.err
}

type resolutionPolicyProvider struct {
	policy hold.AllocationPolicy
	found  bool
	err    error
}

func (r resolutionPolicyProvider) PolicyFor(context.Context, account.Account, hold.HoldType) (hold.AllocationPolicy, bool, error) {
	return r.policy, r.found, r.err
}

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RESOLVE PENDING HOLD", func() {
	Describe("#Execute", func() {
		Context("success cases", func() {
			It("should allocate the full requested amount using eligible funds and the matching policy", func() {
				sut, repo, box, input, ctx := newResolutionExecution(hold.ActionAllocate, 1000, 500, false, false)

				output, err := sut.Execute(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(output.Instruction.Status).To(Equal(hold.StatusAllocated))
				Expect(output.Instruction.Allocated.Amount()).To(Equal(int64(500)))
				Expect(output.Instruction.Pending.Amount()).To(BeZero())
				Expect(output.Instruction.Version).To(Equal(uint64(2)))
				Expect(repo.decision.PolicySnapshot).NotTo(BeEmpty())
				Expect(repo.histories).To(HaveLen(1))
				Expect(box.saved).NotTo(BeNil())
			})

			It("should partially allocate only when both the instruction and policy permit it", func() {
				sut, _, _, input, ctx := newResolutionExecution(hold.ActionPartiallyAllocate, 300, 500, true, true)

				output, err := sut.Execute(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(output.Instruction.Status).To(Equal(hold.StatusPartiallyAllocated))
				Expect(output.Instruction.Allocated.Amount()).To(Equal(int64(300)))
				Expect(output.Instruction.Pending.Amount()).To(Equal(int64(200)))
			})

			It("should replay only a decision with the same canonical fingerprint", func() {
				input := dto.ResolvePendingHoldInput{HoldID: "hold-id", DecisionID: "decision-id", AuthorizationReference: "auth-ref",
					Action: hold.ActionReject, Reason: "review rejected", ExpectedVersion: 1}
				principal := security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}}
				fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{
					input.HoldID, input.DecisionID, input.AuthorizationReference, principal.Subject, string(input.Action),
					input.PolicyVersion, "1", input.Reason, string(input.RequestedAmount.Currency()), "0",
				}, "\x00"))))
				requested, _ := money.NewMoney(500, money.USD)
				zero, _ := money.NewMoney(0, money.USD)
				repo := &resolutionHoldRepo{
					instruction: &hold.Instruction{ID: "hold-id", Requested: requested, Allocated: zero, Pending: requested,
						Released: zero, Status: hold.StatusRejected, Version: 2},
					decision: &hold.Decision{HoldID: "hold-id", IdempotencyKey: "decision-id", RequestFingerprint: fingerprint},
				}
				ctx := security.WithPrincipal(context.Background(), principal)
				sut := command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, repo, nil, &resolutionOutboxRepo{},
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				output, err := sut.Execute(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(output.IdempotentReplay).To(BeTrue())
				Expect(output.Decision).To(BeIdenticalTo(repo.decision))
				Expect(repo.histories).To(BeEmpty())
			})
		})
		Context("error cases", func() {
			It("should reject a hold that is missing or no longer pending at the expected version", func() {
				missing, _, _, input, ctx := newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				missingHoldRepo := &resolutionHoldRepo{}
				missing = command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, missingHoldRepo, nil, &resolutionOutboxRepo{},
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err := missing.Execute(ctx, input)
				Expect(err).To(MatchError(hold.ErrInvalidResolution))

				stale, repo, _, input, ctx := newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				repo.instruction.Version++
				_, err = stale.Execute(ctx, input)
				Expect(errors.Is(err, hold.ErrHoldStateConflict)).To(BeTrue(), "err: %v", err)
			})

			It("should preserve repository failures when loading the decision or hold", func() {
				_, repo, _, input, ctx := newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				lookupErr := errors.New("decision lookup failed")
				repo.decisionErr = lookupErr
				sut, _, _, _, _ := newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				sut = command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, repo, nil, &resolutionOutboxRepo{},
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err := sut.Execute(ctx, input)
				Expect(errors.Is(err, lookupErr)).To(BeTrue(), "err: %v", err)

				_, repo, _, input, ctx = newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				holdErr := errors.New("hold lookup failed")
				repo.findErr = holdErr
				sut = command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, repo, nil, &resolutionOutboxRepo{},
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err = sut.Execute(ctx, input)
				Expect(errors.Is(err, holdErr)).To(BeTrue())
			})

			It("should reject requests whose amount exceeds the pending amount or uses another currency", func() {
				sut, _, _, input, ctx := newResolutionExecution(hold.ActionAllocate, 1000, 500, false, false)
				input.RequestedAmount, _ = money.NewMoney(501, money.BRL)
				_, err := sut.Execute(ctx, input)
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())

				sut, _, _, input, ctx = newResolutionExecution(hold.ActionAllocate, 1000, 500, false, false)
				input.RequestedAmount, _ = money.NewMoney(500, money.USD)
				_, err = sut.Execute(ctx, input)
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())
			})

			It("should reject an account that cannot retain funds", func() {
				sut, repo, box, input, ctx := newResolutionExecution(hold.ActionAllocate, 1000, 500, false, false)
				ineligible := testResolutionAccount()
				ineligible.HoldsEligible = false
				sut = command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{value: ptrResolutionAccount(ineligible)},
					resolutionTransactionRepo{balance: 1000}, repo, resolutionPolicyProvider{policy: testResolutionPolicy("policy-v1", false), found: true}, box,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err := sut.Execute(ctx, input)
				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should return failures while persisting the decision or outbox event", func() {
				sut, repo, _, input, ctx := newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				writeErr := errors.New("decision write failed")
				repo.saveErr = writeErr
				_, err := sut.Execute(ctx, input)
				Expect(errors.Is(err, writeErr)).To(BeTrue(), "err: %v", err)

				sut, repo, _, input, ctx = newResolutionExecution(hold.ActionReject, 0, 0, false, false)
				outboxRepo := &resolutionOutboxRepo{err: errors.New("outbox write failed")}
				outboxRepo.onError = func() { repo.decision = nil }
				sut = command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, repo, nil, outboxRepo,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err = sut.Execute(ctx, input)
				Expect(errors.Is(err, outboxRepo.err)).To(BeTrue())
			})

			It("should reject a full allocation when the evaluated funds are insufficient", func() {
				sut, _, _, input, ctx := newResolutionExecution(hold.ActionAllocate, 300, 500, true, true)

				_, err := sut.Execute(ctx, input)

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should reject a partial allocation when the instruction disallows it", func() {
				sut, _, _, input, ctx := newResolutionExecution(hold.ActionPartiallyAllocate, 300, 500, false, true)

				_, err := sut.Execute(ctx, input)

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should reject a policy version that is unavailable or does not match", func() {
				_, repo, _, input, ctx := newResolutionExecution(hold.ActionAllocate, 1000, 500, true, true)
				sut := command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{value: ptrResolutionAccount(testResolutionAccount())},
					resolutionTransactionRepo{balance: 1000}, repo,
					resolutionPolicyProvider{policy: testResolutionPolicy("other-version", true), found: true},
					&resolutionOutboxRepo{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				_, err := sut.Execute(ctx, input)

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should reject a resolution without the required fields", func() {
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
				sut := command.NewResolvePendingHold(nil, nil, nil, nil, nil, nil,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				_, err := sut.Execute(ctx, dto.ResolvePendingHoldInput{})

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should reject an unsupported resolution action", func() {
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
				sut := command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, nil, nil, nil,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				input := dto.ResolvePendingHoldInput{HoldID: "hold-id", DecisionID: "decision-id", AuthorizationReference: "auth-ref",
					Reason: "reviewed", ExpectedVersion: 1, Action: "unsupported"}

				_, err := sut.Execute(ctx, input)

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should require an authenticated principal with resolution permission", func() {
				sut := command.NewResolvePendingHold(nil, nil, nil, nil, nil, nil,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				_, err := sut.Execute(context.Background(), dto.ResolvePendingHoldInput{})

				Expect(err).To(MatchError(command.ErrUnauthorizedHoldResolution))
			})

			It("should reject a principal without the technical permission", func() {
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a"})
				sut := command.NewResolvePendingHold(nil, nil, nil, nil, nil, nil,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				_, err := sut.Execute(ctx, dto.ResolvePendingHoldInput{})

				Expect(err).To(MatchError(command.ErrUnauthorizedHoldResolution))
			})

			It("should persist a versioned payload without sensitive decision fields", func() {
				requested, _ := money.NewMoney(500, money.USD)
				zero, _ := money.NewMoney(0, money.USD)
				repo := &resolutionHoldRepo{instruction: &hold.Instruction{ID: "hold-id", AccountID: "account-id", Currency: "USD", Type: hold.Judicial,
					Requested: requested, Allocated: zero, Pending: requested, Released: zero, Status: hold.StatusPendingReview, Version: 1}}
				box := &resolutionOutboxRepo{}
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
				sut := command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{}, resolutionTransactionRepo{}, repo, nil, box,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
				_, err := sut.Execute(ctx, dto.ResolvePendingHoldInput{HoldID: "hold-id", DecisionID: "decision-id", AuthorizationReference: "auth-ref",
					Action: hold.ActionReject, Reason: "sensitive rationale", ExpectedVersion: 1})
				Expect(err).NotTo(HaveOccurred())
				Expect(repo.histories).To(HaveLen(1))
				Expect(repo.histories[0].EventType).To(Equal("DECISION_RESOLVED"))
				Expect(box.saved).NotTo(BeNil())
				Expect(box.saved.EventType).To(Equal(outbox.HoldDecisionResolved))
				var payload map[string]any
				Expect(json.Unmarshal(box.saved.Payload, &payload)).To(Succeed())
				Expect(payload["event_version"]).To(Equal(float64(1)))
				Expect(payload["decision_id"]).To(Equal("decision-id"))
				Expect(payload["requested_amount"]).To(Equal(float64(500)))
				Expect(payload).NotTo(HaveKey("reason"))
				Expect(payload).NotTo(HaveKey("authorization_reference"))
				Expect(payload).NotTo(HaveKey("authorized_by"))
			})
		})
	})
})

func newResolutionExecution(action hold.ResolutionAction, balance, requested int64, allowPartial, policyAllowPartial bool) (
	*command.ResolvePendingHold, *resolutionHoldRepo, *resolutionOutboxRepo, dto.ResolvePendingHoldInput, context.Context,
) {
	amountRequested, _ := money.NewMoney(requested, money.BRL)
	zero, _ := money.NewMoney(0, money.BRL)
	accountValue := testResolutionAccount()
	instruction := &hold.Instruction{ID: "hold-id", AccountID: accountValue.ID.String(), Currency: string(money.BRL),
		Type: hold.Judicial, Requested: amountRequested, Allocated: zero, Pending: amountRequested, Released: zero,
		Status: hold.StatusPendingReview, Version: 1, AllowPartial: allowPartial}
	repo := &resolutionHoldRepo{instruction: instruction}
	box := &resolutionOutboxRepo{}
	policy := testResolutionPolicy("policy-v1", policyAllowPartial)
	sut := command.NewResolvePendingHold(resolutionUOW{}, resolutionAccountRepo{value: &accountValue},
		resolutionTransactionRepo{balance: balance}, repo, resolutionPolicyProvider{policy: policy, found: true}, box,
		adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
	inputAmount, _ := money.NewMoney(requested, money.BRL)
	policyVersion := ""
	if action == hold.ActionAllocate || action == hold.ActionPartiallyAllocate {
		policyVersion = policy.Version
	}
	input := dto.ResolvePendingHoldInput{HoldID: instruction.ID, DecisionID: "decision-id", AuthorizationReference: "auth-ref",
		Reason: "reviewed", Action: action, PolicyVersion: policyVersion, ExpectedVersion: instruction.Version,
		RequestedAmount: inputAmount}
	ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
	return sut, repo, box, input, ctx
}

func testResolutionAccount() account.Account {
	id, _ := entity.NewID("123e4567-e89b-42d3-a456-426614174000")
	return account.Account{ID: id, Currency: money.BRL, HoldsEligible: true}
}

func ptrResolutionAccount(value account.Account) *account.Account {
	return &value
}

func testResolutionPolicy(version string, allowPartial bool) hold.AllocationPolicy {
	return hold.AllocationPolicy{Version: version, TypeOrder: []hold.HoldType{hold.Judicial}, AllowPartial: allowPartial,
		UnresolvedConflict: hold.PendingReviewOnConflict}
}

var _ application.UnitOfWork = resolutionUOW{}
var _ hold.Repository = (*resolutionHoldRepo)(nil)
var _ outbox.Repository = (*resolutionOutboxRepo)(nil)
var _ account.Repository = resolutionAccountRepo{}
var _ transaction.Repository = resolutionTransactionRepo{}
var _ command.HoldPolicyProvider = resolutionPolicyProvider{}
