//go:build unit

package model_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: HOLD", func() {
	Describe("#ToHoldModel and #ToHoldDomain", func() {
		Context("success cases", func() {
			It("should preserve version, partial allocation permission, and rejected status", func() {
				// Arrange.
				zero, err := money.NewMoney(0, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				requested, err := money.NewMoney(450, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				instruction := &hold.Instruction{ID: "hold-1", AccountID: "account-1", Currency: "BRL",
					Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: requested,
					Allocated: zero, Pending: requested, Released: zero, Status: hold.StatusRejected,
					Version: 4, AllowPartial: true, RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

				// Act.
				persisted, err := model.ToHoldModel(instruction)
				Expect(err).NotTo(HaveOccurred())
				decoded, err := model.ToHoldDomain(persisted)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(decoded.Version).To(Equal(uint64(4)))
				Expect(decoded.AllowPartial).To(BeTrue())
				Expect(decoded.Status).To(Equal(hold.StatusRejected))
			})
		})
	})

	Describe("#ToHoldDecisionModel and #ToHoldDecisionDomain", func() {
		Context("success cases", func() {
			It("should preserve authorization, policy snapshot, and replay result", func() {
				// Arrange.
				requested, err := money.NewMoney(450, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				allocated, err := money.NewMoney(0, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				decision := hold.Decision{ID: "decision-1", HoldID: "hold-1", IdempotencyKey: "key-1",
					RequestFingerprint:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					AuthorizationReference: "auth-1", AuthorizedBy: "operator-1", Action: hold.ActionReject,
					Reason: "not applicable", ResultStatus: string(hold.StatusRejected), PolicySnapshot: []byte(`{"version":"v1"}`),
					ExpectedVersion: 3, Requested: requested, ResultAllocated: allocated, ResultPending: requested,
					CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

				// Act.
				persisted, err := model.ToHoldDecisionModel(decision)
				Expect(err).NotTo(HaveOccurred())
				decoded, err := model.ToHoldDecisionDomain(persisted, money.BRL)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(decoded.AuthorizedBy).To(Equal(decision.AuthorizedBy))
				Expect(decoded.PolicySnapshot).To(Equal(decision.PolicySnapshot))
				Expect(decoded.ResultStatus).To(Equal(decision.ResultStatus))
				Expect(decoded.ResultPending).To(Equal(decision.ResultPending))
			})
		})
	})
})
