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

	Describe("#Instruction", func() {
		Context("success cases", func() {
			It("should validate allocated, rejected, and released lifecycle states", func() {
				requested := mustMoney(100, money.BRL)
				zero := mustMoney(0, money.BRL)
				cases := []hold.Instruction{
					{ID: "h-allocated", AccountID: "a1", Currency: "BRL", Origin: "issuer", Reference: "allocated", Type: hold.Judicial,
						Requested: requested, Allocated: requested, Pending: zero, Released: zero, Status: hold.StatusAllocated},
					{ID: "h-rejected", AccountID: "a1", Currency: "BRL", Origin: "issuer", Reference: "rejected", Type: hold.Judicial,
						Requested: requested, Allocated: zero, Pending: requested, Released: zero, Status: hold.StatusRejected},
					{ID: "h-released", AccountID: "a1", Currency: "BRL", Origin: "issuer", Reference: "released", Type: hold.Judicial,
						Requested: requested, Allocated: requested, Pending: zero, Released: requested, Status: hold.StatusReleased},
				}
				for _, instruction := range cases {
					Expect(instruction.Validate()).To(Succeed(), instruction.Status)
				}
			})

			It("should fingerprint immutable content and allow a bounded partial release", func() {
				requested := mustMoney(100, money.BRL)
				instruction := hold.Instruction{ID: "h1", AccountID: "a1", Currency: "BRL", Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: requested, Allocated: mustMoney(80, money.BRL), Pending: mustMoney(20, money.BRL), Released: mustMoney(0, money.BRL), Status: hold.StatusPartiallyAllocated}
				fingerprint := instruction.Fingerprint()

				Expect(instruction.Validate()).To(Succeed())
				Expect(instruction.Identity()).To(Equal("issuer:ref-1"))
				Expect(fingerprint).To(HaveLen(64))
				Expect(instruction.Release(mustMoney(30, money.BRL))).To(Succeed())
				Expect(instruction.Released.Amount()).To(Equal(int64(30)))
				Expect(instruction.Status).To(Equal(hold.StatusPartiallyAllocated))
			})

			It("should apply an allocation decision while keeping pending funds unreserved", func() {
				instruction := hold.Instruction{ID: "h1", AccountID: "a1", Currency: "BRL", Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: mustMoney(100, money.BRL), Released: mustMoney(0, money.BRL)}
				decision := hold.HoldDecision{RequestID: "h1", Requested: mustMoney(100, money.BRL), Allocated: mustMoney(0, money.BRL), Pending: mustMoney(100, money.BRL), Status: hold.PendingReview}

				Expect(instruction.ApplyDecision(decision)).To(Succeed())
				Expect(instruction.Validate()).To(Succeed())
				Expect(instruction.Status).To(Equal(hold.StatusPendingReview))
			})
		})

		Context("error cases", func() {
			It("should reject release beyond the remaining allocation and mismatched currencies", func() {
				instruction := hold.Instruction{Allocated: mustMoney(80, money.BRL), Released: mustMoney(30, money.BRL)}
				Expect(instruction.Release(mustMoney(51, money.BRL))).To(MatchError(ContainSubstring("exceeds retained")))
				Expect(instruction.Release(mustMoney(1, money.USD))).To(MatchError(hold.ErrInvalidRelease))
			})

			It("should reject inconsistent persisted amounts and mismatched decisions", func() {
				instruction := hold.Instruction{ID: "h1", AccountID: "a1", Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: mustMoney(100, money.BRL), Allocated: mustMoney(80, money.BRL), Pending: mustMoney(10, money.BRL), Released: mustMoney(0, money.BRL), Status: hold.StatusPartiallyAllocated}
				Expect(instruction.Validate()).To(HaveOccurred())
				instruction.Pending = mustMoney(20, money.BRL)
				Expect(instruction.ApplyDecision(hold.HoldDecision{RequestID: "other", Requested: mustMoney(100, money.BRL)})).To(HaveOccurred())
			})
		})
	})

	Describe("#Resolve", func() {
		Context("success cases", func() {
			It("should apply full, partial, and keep-pending decisions with a new version", func() {
				cases := []struct {
					action    hold.ResolutionAction
					allocated int64
					partial   bool
					status    hold.HoldStatus
					result    int64
				}{
					{hold.ActionAllocate, 100, false, hold.StatusAllocated, 100},
					{hold.ActionPartiallyAllocate, 40, true, hold.StatusPartiallyAllocated, 40},
					{hold.ActionKeepPending, 0, false, hold.StatusPendingReview, 0},
				}
				for _, testCase := range cases {
					instruction := pendingInstruction(testCase.partial)
					err := instruction.Resolve(hold.Resolution{Action: testCase.action, ExpectedStatus: hold.StatusPendingReview, ExpectedVersion: 3, Allocated: mustMoney(testCase.allocated, money.BRL), PolicyVersion: "policy-v1"})
					Expect(err).NotTo(HaveOccurred())
					Expect(instruction.Status).To(Equal(testCase.status))
					Expect(instruction.Allocated.Amount()).To(Equal(testCase.result))
					Expect(instruction.Released.Amount()).To(BeZero())
					Expect(instruction.Version).To(Equal(uint64(4)))
					Expect(instruction.Validate()).To(Succeed())
				}
			})

			It("should reject a pending hold without recording a release", func() {
				instruction := pendingInstruction(false)

				err := instruction.Resolve(hold.Resolution{Action: hold.ActionReject, ExpectedStatus: hold.StatusPendingReview, ExpectedVersion: 3, Allocated: mustMoney(0, money.BRL)})

				Expect(err).NotTo(HaveOccurred())
				Expect(instruction.Status).To(Equal(hold.StatusRejected))
				Expect(instruction.Allocated.Amount()).To(BeZero())
				Expect(instruction.Pending.Amount()).To(Equal(int64(100)))
				Expect(instruction.Released.Amount()).To(BeZero())
				Expect(instruction.Validate()).To(Succeed())
			})
		})

		Context("error cases", func() {
			It("should reject exhausted versions, invalid amounts, incomplete allocations, and unknown actions", func() {
				instruction := pendingInstruction(false)
				instruction.Version = ^uint64(0)
				err := instruction.Resolve(hold.Resolution{Action: hold.ActionAllocate, ExpectedStatus: hold.StatusPendingReview,
					ExpectedVersion: instruction.Version, Allocated: mustMoney(100, money.BRL)})
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())

				instruction = pendingInstruction(false)
				err = instruction.Resolve(hold.Resolution{Action: hold.ActionAllocate, ExpectedStatus: hold.StatusPendingReview,
					ExpectedVersion: 3, Allocated: mustMoney(99, money.BRL)})
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())

				instruction = pendingInstruction(true)
				err = instruction.Resolve(hold.Resolution{Action: hold.ActionPartiallyAllocate, ExpectedStatus: hold.StatusPendingReview,
					ExpectedVersion: 3, Allocated: mustMoney(100, money.BRL)})
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())

				instruction = pendingInstruction(false)
				err = instruction.Resolve(hold.Resolution{Action: "UNKNOWN", ExpectedStatus: hold.StatusPendingReview,
					ExpectedVersion: 3, Allocated: mustMoney(0, money.BRL)})
				Expect(errors.Is(err, hold.ErrInvalidResolution)).To(BeTrue())
			})

			It("should reject stale state or version without mutating the instruction", func() {
				instruction := pendingInstruction(true)
				before := instruction
				err := instruction.Resolve(hold.Resolution{Action: hold.ActionAllocate, ExpectedStatus: hold.StatusAllocated, ExpectedVersion: 2, Allocated: mustMoney(100, money.BRL)})
				Expect(err).To(MatchError(hold.ErrHoldStateConflict))
				Expect(instruction).To(Equal(before))
			})

			It("should reject partial allocation when the instruction forbids it and rejection never releases funds", func() {
				instruction := pendingInstruction(false)
				before := instruction
				err := instruction.Resolve(hold.Resolution{Action: hold.ActionPartiallyAllocate, ExpectedStatus: hold.StatusPendingReview, ExpectedVersion: 3, Allocated: mustMoney(40, money.BRL)})
				Expect(err).To(MatchError(hold.ErrInvalidResolution))
				Expect(instruction).To(Equal(before))
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

func pendingInstruction(allowPartial bool) *hold.Instruction {
	return &hold.Instruction{
		ID: "hold-1", AccountID: "account-1", Currency: "BRL", Origin: "issuer", Reference: "ref-1",
		Type: hold.Judicial, Requested: mustMoney(100, money.BRL), Allocated: mustMoney(0, money.BRL),
		Pending: mustMoney(100, money.BRL), Released: mustMoney(0, money.BRL), Status: hold.StatusPendingReview,
		Version: 3, AllowPartial: allowPartial,
	}
}

func mustMoney(amount int64, currency money.Currency) money.Money {
	value, err := money.NewMoney(amount, currency)
	Expect(err).NotTo(HaveOccurred())
	return value
}
