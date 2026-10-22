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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type holdUOW struct{ application.UnitOfWork }

func (u holdUOW) WithRetryableTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type holdRepositoryFake struct {
	hold.Repository
	existing *hold.Instruction
	saved    *hold.Instruction
}

func (r *holdRepositoryFake) FindByIdentity(context.Context, string, string) (*hold.Instruction, error) {
	return r.existing, nil
}

func (r *holdRepositoryFake) Save(_ context.Context, instruction *hold.Instruction) error {
	r.saved = instruction
	return nil
}
func (*holdRepositoryFake) SaveHistory(context.Context, hold.History) error { return nil }
func (*holdRepositoryFake) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return nil, nil
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

type holdPolicyFake struct {
	found  bool
	policy hold.AllocationPolicy
}

func (r holdPolicyFake) PolicyFor(context.Context, account.Account, hold.HoldType) (hold.AllocationPolicy, bool, error) {
	return r.policy, r.found, nil
}

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
				sut := command.NewCreateHold(holdUOW{}, nil, nil, repo, nil, nil)

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
				sut := command.NewCreateHold(holdUOW{}, holdAccountRepoFake{value: accountEntity}, holdTransactionRepoFake{}, repo, holdPolicyFake{}, holdOutboxRepoFake{})
				input := dto.CreateHoldInput{AccountID: accountID.String(), Currency: string(money.BRL), Origin: "origin", Reference: "unconfigured-policy", Type: hold.Judicial, Requested: requested}

				result, err := sut.Execute(context.Background(), input)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Instruction.Status).To(Equal(hold.StatusPendingReview))
				Expect(result.Instruction.Allocated.Amount()).To(BeZero())
				Expect(result.Instruction.Pending).To(Equal(requested))
				Expect(repo.saved).To(BeIdenticalTo(result.Instruction))
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
				sut := command.NewCreateHold(holdUOW{}, nil, nil, &holdRepositoryFake{existing: stored}, nil, nil)

				result, err := sut.Execute(context.Background(), input)

				Expect(result).To(BeNil())
				Expect(err).To(MatchError(hold.ErrHoldConflict))
			})
		})
	})
})
