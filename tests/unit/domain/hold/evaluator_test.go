//go:build unit
// +build unit

package hold_test

import (
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: HOLD :: EVALUATOR", func() {
	Describe("#Evaluate", func() {
		Context("success cases", func() {
			It("should exclude active allocations before allocating a new request", func() {
				input := evaluationInput(100, []hold.ActiveAllocation{{HoldID: "old", Type: hold.Contractual, Allocated: mustMoney(70, money.BRL)}}, []hold.HoldRequest{request("new", hold.Judicial, 50)}, validPolicy())

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions).To(HaveLen(1))
				Expect(decisions[0].Status).To(Equal(hold.PartiallyAllocated))
				Expect(decisions[0].Allocated.Amount()).To(Equal(int64(30)))
				Expect(decisions[0].Pending.Amount()).To(Equal(int64(20)))
			})

			It("should allocate a single request partially when the policy allows it", func() {
				policy := validPolicy()
				input := evaluationInput(60, nil, []hold.HoldRequest{request("r1", hold.Judicial, 100)}, policy)

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions[0].Status).To(Equal(hold.PartiallyAllocated))
				Expect(decisions[0].Allocated.Amount()).To(Equal(int64(60)))
				Expect(decisions[0].Pending.Amount()).To(Equal(int64(40)))
			})

			It("should apply the configured type order and retain its version", func() {
				policy := validPolicy()
				policy.Version = "ops-v2"
				policy.TypeOrder = []hold.HoldType{hold.Regulatory, hold.Judicial}
				policy.AllowPartial = false
				input := evaluationInput(100, nil, []hold.HoldRequest{request("judicial", hold.Judicial, 80), request("regulatory", hold.Regulatory, 80)}, policy)

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions[0].Status).To(Equal(hold.PendingReview))
				Expect(decisions[1].Status).To(Equal(hold.Allocated))
				Expect(decisions[1].Allocated.Amount()).To(Equal(int64(80)))
				Expect(decisions[0].PolicyVersion).To(Equal("ops-v2"))
			})

			It("should route unresolved requests of the same type to review", func() {
				input := evaluationInput(100, nil, []hold.HoldRequest{request("judicial-a", hold.Judicial, 80), request("judicial-b", hold.Judicial, 80)}, validPolicy())

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions[0].Status).To(Equal(hold.PendingReview))
				Expect(decisions[1].Status).To(Equal(hold.PendingReview))
				Expect(decisions[0].ConflictIDs).To(ConsistOf("judicial-b"))
				Expect(decisions[1].ConflictIDs).To(ConsistOf("judicial-a"))
				Expect(decisions[0].Allocated.Amount()).To(BeZero())
			})

			It("should leave input slices and prior decisions unchanged", func() {
				allocations := []hold.ActiveAllocation{{HoldID: "old", Type: hold.Contractual, Allocated: mustMoney(10, money.BRL)}}
				requests := []hold.HoldRequest{request("new", hold.Judicial, 20)}
				policy := validPolicy()
				input := evaluationInput(100, allocations, requests, policy)
				prior := hold.HoldDecision{RequestID: "prior", PolicyVersion: "ops-0", Allocated: mustMoney(1, money.BRL), ConflictIDs: []string{"historic"}, Reason: "KEPT"}

				first, err := hold.Evaluate(input)
				Expect(err).NotTo(HaveOccurred())
				second, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(first).To(Equal(second))
				Expect(allocations[0].Allocated.Amount()).To(Equal(int64(10)))
				Expect(requests[0].Requested.Amount()).To(Equal(int64(20)))
				Expect(policy.TypeOrder).To(Equal([]hold.HoldType{hold.Judicial, hold.Regulatory}))
				Expect(prior.ConflictIDs).To(Equal([]string{"historic"}))
				Expect(prior.Reason).To(Equal("KEPT"))
			})

			It("should route an unconfigured type and competing requests to review", func() {
				input := evaluationInput(100, nil, []hold.HoldRequest{request("judicial", hold.Judicial, 80), request("guarantee", hold.Guarantee, 80)}, validPolicy())

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions[0].Status).To(Equal(hold.PendingReview))
				Expect(decisions[1].Status).To(Equal(hold.PendingReview))
				Expect(decisions[0].ConflictIDs).To(ConsistOf("guarantee"))
				Expect(decisions[1].Reason).To(Equal("UNRESOLVED_TYPE_PRIORITY"))
			})

			It("should leave a request pending when partial allocation is disabled", func() {
				policy := validPolicy()
				policy.AllowPartial = false
				input := evaluationInput(20, nil, []hold.HoldRequest{request("r1", hold.Judicial, 100)}, policy)

				decisions, err := hold.Evaluate(input)

				Expect(err).NotTo(HaveOccurred())
				Expect(decisions[0].Status).To(Equal(hold.PendingReview))
				Expect(decisions[0].Pending.Amount()).To(Equal(int64(100)))
			})
		})

		Context("error cases", func() {
			It("should reject active allocations greater than eligible funds", func() {
				input := evaluationInput(50, []hold.ActiveAllocation{{HoldID: "old", Type: hold.Judicial, Allocated: mustMoney(60, money.BRL)}}, nil, validPolicy())

				_, err := hold.Evaluate(input)

				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, hold.ErrInvalidEvaluation)).To(BeTrue())
			})
		})
	})
})

func evaluationInput(funds int64, allocations []hold.ActiveAllocation, requests []hold.HoldRequest, policy hold.AllocationPolicy) hold.HoldEvaluationInput {
	return hold.HoldEvaluationInput{EligibleFunds: mustMoney(funds, money.BRL), ActiveAllocations: allocations, Requests: requests, Policy: policy}
}

func request(id string, holdType hold.HoldType, amount int64) hold.HoldRequest {
	return hold.HoldRequest{ID: id, Type: holdType, Requested: mustMoney(amount, money.BRL)}
}
