//go:build unit
// +build unit

package command_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/application/security"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
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
}

func (r *resolutionHoldRepo) FindDecision(context.Context, string) (*hold.Decision, error) {
	return r.decision, nil
}
func (r *resolutionHoldRepo) FindByID(context.Context, string) (*hold.Instruction, error) {
	return r.instruction, nil
}
func (r *resolutionHoldRepo) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return nil, nil
}
func (r *resolutionHoldRepo) SaveDecision(_ context.Context, decision hold.Decision) error {
	r.decision = &decision
	return nil
}
func (r *resolutionHoldRepo) Update(_ context.Context, instruction *hold.Instruction) error {
	r.instruction = instruction
	return nil
}
func (r *resolutionHoldRepo) SaveHistory(_ context.Context, history hold.History) error {
	r.histories = append(r.histories, history)
	return nil
}

type resolutionOutboxRepo struct {
	outbox.Repository
	saved *outbox.Outbox
}

func (r *resolutionOutboxRepo) Save(_ context.Context, item *outbox.Outbox) error {
	r.saved = item
	return nil
}

type resolutionAccountRepo struct{ account.Repository }
type resolutionTransactionRepo struct{ transaction.Repository }

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RESOLVE PENDING HOLD", func() {
	Describe("#Execute", func() {
		Context("success cases", func() {
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
			It("should reject a resolution without the required fields", func() {
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
				sut := command.NewResolvePendingHold(nil, nil, nil, nil, nil, nil,
					adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})

				_, err := sut.Execute(ctx, dto.ResolvePendingHoldInput{})

				Expect(err).To(MatchError(hold.ErrInvalidResolution))
			})

			It("should reject an unsupported resolution action", func() {
				ctx := security.WithPrincipal(context.Background(), security.Principal{Subject: "service-a", Permissions: []string{security.ResolveFundsHold}})
				sut := command.NewResolvePendingHold(resolutionUOW{}, nil, nil, nil, nil, nil,
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

var _ application.UnitOfWork = resolutionUOW{}
var _ hold.Repository = (*resolutionHoldRepo)(nil)
var _ outbox.Repository = (*resolutionOutboxRepo)(nil)
var _ account.Repository = resolutionAccountRepo{}
var _ transaction.Repository = resolutionTransactionRepo{}
