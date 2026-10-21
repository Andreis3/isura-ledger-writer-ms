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

var _ = Describe("INTERNAL :: DOMAIN :: HOLD :: HOLD", func() {
	Describe("#Validate", func() {
		Context("success cases", func() {
			It("should accept every supported hold type", func() {
				for _, holdType := range []hold.HoldType{hold.Judicial, hold.Regulatory, hold.Guarantee, hold.Contractual, hold.Operational} {
					Expect(holdType.Validate()).To(Succeed())
				}
			})
		})

		Context("error cases", func() {
			It("should reject an unknown hold type", func() {
				err := hold.HoldType("UNKNOWN").Validate()
				Expect(err).To(MatchError(ContainSubstring("invalid hold type")))
				Expect(errors.Is(err, hold.ErrInvalidHoldType)).To(BeTrue())
			})
		})
	})

	Describe("#HoldRequest.Validate", func() {
		Context("success cases", func() {
			It("should accept a request with a valid positive amount", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				Expect((hold.HoldRequest{ID: "req-1", Type: hold.Judicial, Requested: amount}).Validate()).To(Succeed())
			})
		})

		Context("error cases", func() {
			It("should reject missing identifiers, invalid types, zero amounts, and invalid currencies", func() {
				valid := mustMoney(100, money.BRL)
				zero := mustMoney(0, money.BRL)
				invalidCurrency := money.Money{}
				Expect((hold.HoldRequest{Type: hold.Judicial, Requested: valid}).Validate()).To(HaveOccurred())
				Expect((hold.HoldRequest{ID: "req-2", Type: "OTHER", Requested: valid}).Validate()).To(HaveOccurred())
				Expect((hold.HoldRequest{ID: "req-3", Type: hold.Judicial, Requested: zero}).Validate()).To(HaveOccurred())
				Expect((hold.HoldRequest{ID: "req-4", Type: hold.Judicial, Requested: invalidCurrency}).Validate()).To(HaveOccurred())
			})
		})
	})

	Describe("#ActiveAllocation.Validate", func() {
		Context("success cases", func() {
			It("should accept a nonnegative allocation with a valid currency", func() {
				amount, err := money.NewMoney(0, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				Expect((hold.ActiveAllocation{HoldID: "hold-1", Type: hold.Regulatory, Allocated: amount}).Validate()).To(Succeed())
			})
		})

		Context("error cases", func() {
			It("should reject missing identifiers, invalid types, and invalid money", func() {
				valid := mustMoney(10, money.BRL)
				negative, err := mustMoney(1, money.BRL).Subtract(mustMoney(2, money.BRL))
				Expect(err).NotTo(HaveOccurred())
				Expect((hold.ActiveAllocation{Type: hold.Judicial, Allocated: valid}).Validate()).To(HaveOccurred())
				Expect((hold.ActiveAllocation{HoldID: "hold-2", Type: "OTHER", Allocated: valid}).Validate()).To(HaveOccurred())
				Expect((hold.ActiveAllocation{HoldID: "hold-3", Type: hold.Judicial, Allocated: negative}).Validate()).To(HaveOccurred())
				Expect((hold.ActiveAllocation{HoldID: "hold-4", Type: hold.Judicial, Allocated: money.Money{}}).Validate()).To(HaveOccurred())
			})
		})
	})

	Describe("#AllocationPolicy.Validate", func() {
		Context("success cases", func() {
			It("should accept a complete versioned policy", func() {
				Expect(validPolicy().Validate()).To(Succeed())
			})
		})

		Context("error cases", func() {
			It("should reject incomplete, invalid, and ambiguous policies", func() {
				policy := validPolicy()
				policy.Version = ""
				Expect(policy.Validate()).To(HaveOccurred())
				policy = validPolicy()
				policy.TypeOrder = nil
				Expect(policy.Validate()).To(HaveOccurred())
				policy = validPolicy()
				policy.TypeOrder = []hold.HoldType{hold.Judicial, "OTHER"}
				Expect(policy.Validate()).To(HaveOccurred())
				policy = validPolicy()
				policy.TypeOrder = []hold.HoldType{hold.Judicial, hold.Judicial}
				Expect(policy.Validate()).To(HaveOccurred())
				policy = validPolicy()
				policy.UnresolvedConflict = "AUTOMATIC"
				Expect(policy.Validate()).To(HaveOccurred())
			})
		})
	})

	Describe("#HoldEvaluationInput.Validate", func() {
		Context("success cases", func() {
			It("should accept a consistent snapshot in one currency", func() {
				funds := mustMoney(100, money.BRL)
				request := hold.HoldRequest{ID: "req-1", Type: hold.Judicial, Requested: mustMoney(50, money.BRL)}
				allocation := hold.ActiveAllocation{HoldID: "old-1", Type: hold.Contractual, Allocated: mustMoney(20, money.BRL)}
				input := hold.HoldEvaluationInput{EligibleFunds: funds, Requests: []hold.HoldRequest{request}, ActiveAllocations: []hold.ActiveAllocation{allocation}, Policy: validPolicy()}
				Expect(input.Validate()).To(Succeed())
			})
		})

		Context("error cases", func() {
			It("should reject invalid currencies, mismatches, duplicate requests, and invalid policy", func() {
				input := hold.HoldEvaluationInput{EligibleFunds: money.Money{}, Policy: validPolicy()}
				Expect(input.Validate()).To(HaveOccurred())
				input = hold.HoldEvaluationInput{EligibleFunds: mustMoney(100, money.BRL), Policy: validPolicy(), Requests: []hold.HoldRequest{{ID: "r", Type: hold.Judicial, Requested: mustMoney(10, money.USD)}}}
				Expect(input.Validate()).To(HaveOccurred())
				input = hold.HoldEvaluationInput{EligibleFunds: mustMoney(100, money.BRL), Policy: validPolicy(), Requests: []hold.HoldRequest{{ID: "r", Type: hold.Judicial, Requested: mustMoney(10, money.BRL)}, {ID: "r", Type: hold.Regulatory, Requested: mustMoney(20, money.BRL)}}}
				Expect(input.Validate()).To(HaveOccurred())
				input = hold.HoldEvaluationInput{EligibleFunds: mustMoney(100, money.BRL), Policy: validPolicy(), ActiveAllocations: []hold.ActiveAllocation{{HoldID: "a", Type: hold.Judicial, Allocated: mustMoney(1, money.USD)}}}
				Expect(input.Validate()).To(HaveOccurred())
				input = hold.HoldEvaluationInput{EligibleFunds: mustMoney(100, money.BRL), Policy: hold.AllocationPolicy{}}
				Expect(input.Validate()).To(HaveOccurred())
			})

			It("should reject duplicate active allocation identifiers", func() {
				allocation := hold.ActiveAllocation{HoldID: "old-1", Type: hold.Judicial, Allocated: mustMoney(10, money.BRL)}
				input := hold.HoldEvaluationInput{
					EligibleFunds:     mustMoney(100, money.BRL),
					ActiveAllocations: []hold.ActiveAllocation{allocation, allocation},
					Policy:            validPolicy(),
				}

				err := input.Validate()

				Expect(err).To(MatchError(ContainSubstring("duplicate active allocation hold ID")))
			})
		})
	})
})

func validPolicy() hold.AllocationPolicy {
	return hold.AllocationPolicy{Version: "ops-1", TypeOrder: []hold.HoldType{hold.Judicial, hold.Regulatory}, AllowPartial: true, UnresolvedConflict: hold.PendingReviewOnConflict}
}

func mustMoney(amount int64, currency money.Currency) money.Money {
	value, err := money.NewMoney(amount, currency)
	Expect(err).NotTo(HaveOccurred())
	return value
}
