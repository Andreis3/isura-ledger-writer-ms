//go:build unit

package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/andreis3/isura-ledger-ms/internal/application/query"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGetHold(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Get Hold Query Suite")
}

type getHoldRepositoryFake struct {
	hold.Repository
	instruction  *hold.Instruction
	history      []hold.History
	effective    []hold.Instruction
	findErr      error
	historyErr   error
	effectiveErr error
}

func (r *getHoldRepositoryFake) FindByID(context.Context, string) (*hold.Instruction, error) {
	return r.instruction, r.findErr
}

func (r *getHoldRepositoryFake) FindHistory(context.Context, string) ([]hold.History, error) {
	return r.history, r.historyErr
}

func (r *getHoldRepositoryFake) FindEffectiveByAccount(context.Context, string, money.Currency) ([]hold.Instruction, error) {
	return r.effective, r.effectiveErr
}

var _ hold.Repository = (*getHoldRepositoryFake)(nil)

var _ = Describe("INTERNAL :: APPLICATION :: QUERY :: GET HOLD", func() {
	Describe("#ByID", func() {
		Context("success cases", func() {
			It("should return the instruction and its history", func() {
				instruction := &hold.Instruction{ID: "hold-1"}
				history := []hold.History{{HoldID: "hold-1", EventType: "DECISION_RECORDED"}}
				sut := query.NewGetHold(&getHoldRepositoryFake{instruction: instruction, history: history})

				result, resultHistory, err := sut.ByID(context.Background(), "hold-1")

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeIdenticalTo(instruction))
				Expect(resultHistory).To(Equal(history))
			})
			It("should return no result when the instruction does not exist", func() {
				sut := query.NewGetHold(&getHoldRepositoryFake{})

				result, history, err := sut.ByID(context.Background(), "missing")

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(history).To(BeNil())
			})
		})
		Context("error cases", func() {
			It("should add context when the instruction lookup fails", func() {
				cause := errors.New("database unavailable")
				sut := query.NewGetHold(&getHoldRepositoryFake{findErr: cause})

				_, _, err := sut.ByID(context.Background(), "hold-1")

				Expect(err).To(MatchError("find hold: database unavailable"))
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
			It("should add context when the history lookup fails", func() {
				cause := errors.New("history unavailable")
				sut := query.NewGetHold(&getHoldRepositoryFake{instruction: &hold.Instruction{ID: "hold-1"}, historyErr: cause})

				_, _, err := sut.ByID(context.Background(), "hold-1")

				Expect(err).To(MatchError("find hold history: history unavailable"))
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
		})
	})

	Describe("#EffectiveByAccount", func() {
		Context("success cases", func() {
			It("should return the effective holds for an account", func() {
				holds := []hold.Instruction{{ID: "hold-1"}}
				sut := query.NewGetHold(&getHoldRepositoryFake{effective: holds})

				result, err := sut.EffectiveByAccount(context.Background(), "account-1", money.BRL)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(holds))
			})
		})
		Context("error cases", func() {
			It("should add context when effective holds cannot be read", func() {
				cause := errors.New("database unavailable")
				sut := query.NewGetHold(&getHoldRepositoryFake{effectiveErr: cause})

				_, err := sut.EffectiveByAccount(context.Background(), "account-1", money.BRL)

				Expect(err).To(MatchError("find effective holds: database unavailable"))
				Expect(errors.Is(err, cause)).To(BeTrue())
			})
		})
	})
})
