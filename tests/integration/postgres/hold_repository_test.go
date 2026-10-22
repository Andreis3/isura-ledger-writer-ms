//go:build integration

package postgres_test

import (
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	domainhold "github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: HOLD", func() {
	Describe("#Save and #FindByIdentity", func() {
		Context("success cases", func() {
			It("should persist the instruction and append-only history in the caller transaction", func() {
				accountID := insertAccountV7(ctx, tx)
				instruction := allocatedHold(accountID, "origin-a", "reference-a")
				sut := repository.NewHoldRepository(pool)
				txContext := database.WithTx(ctx, tx)

				Expect(sut.Save(txContext, instruction)).To(Succeed())
				found, err := sut.FindByIdentity(txContext, instruction.Origin, instruction.Reference)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(Equal(instruction))

				history := domainhold.History{
					ID: newIDV7(), HoldID: instruction.ID, EventType: "DECISION_RECORDED",
					Requested: amount(900, money.BRL), AllocatedDelta: amount(900, money.BRL),
					Pending: amount(0, money.BRL), ReleasedDelta: amount(0, money.BRL),
					PolicyVersion: "policy-v1", ConflictIDs: []string{}, CreatedAt: time.Now().UTC(),
				}
				Expect(sut.SaveHistory(txContext, history)).To(Succeed())
				loadedHistory, err := sut.FindHistory(txContext, instruction.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(loadedHistory).To(HaveLen(1))
				Expect(loadedHistory[0].AllocatedDelta).To(Equal(history.AllocatedDelta))
				Expect(loadedHistory[0].PolicyVersion).To(Equal(history.PolicyVersion))
			})

			It("should return only holds with an unreleased allocated amount", func() {
				accountID := insertAccountV7(ctx, tx)
				effective := allocatedHold(accountID, "origin-effective", "reference-effective")
				pending := allocatedHold(accountID, "origin-pending", "reference-pending")
				pending.Allocated = amount(0, money.BRL)
				pending.Pending = pending.Requested
				pending.Status = domainhold.StatusPendingReview
				sut := repository.NewHoldRepository(pool)
				txContext := database.WithTx(ctx, tx)
				Expect(sut.Save(txContext, effective)).To(Succeed())
				Expect(sut.Save(txContext, pending)).To(Succeed())

				holds, err := sut.FindEffectiveByAccount(txContext, accountID, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				Expect(holds).To(HaveLen(1))
				Expect(holds[0].ID).To(Equal(effective.ID))
			})

			It("should keep hold eligibility disabled and leave accounting entries untouched", func() {
				accountID := insertAccount(ctx, pool)
				sut := repository.NewHoldRepository(pool)
				instruction := allocatedHold(accountID, "accounting-origin", "accounting-reference")
				var eligible bool
				Expect(pool.QueryRow(ctx, `SELECT holds_eligible FROM accounts WHERE id=$1`, accountID).Scan(&eligible)).To(Succeed())
				Expect(eligible).To(BeFalse())
				var entriesBefore int
				Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE account_id=$1`, accountID).Scan(&entriesBefore)).To(Succeed())
				Expect(sut.Save(ctx, instruction)).To(Succeed())
				var entriesAfter int
				Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE account_id=$1`, accountID).Scan(&entriesAfter)).To(Succeed())
				Expect(entriesAfter).To(Equal(entriesBefore))
			})
		})

		Context("error cases", func() {
			It("should reject a duplicate hold identity", func() {
				accountID := insertAccountV7(ctx, tx)
				sut := repository.NewHoldRepository(pool)
				first := allocatedHold(accountID, "duplicate-origin", "duplicate-reference")
				second := allocatedHold(accountID, "duplicate-origin", "duplicate-reference")
				Expect(sut.Save(database.WithTx(ctx, tx), first)).To(Succeed())

				err := sut.Save(database.WithTx(ctx, tx), second)
				Expect(errors.Is(err, domainhold.ErrHoldConflict)).To(BeTrue())
			})

			It("should not leave hold state after the caller rolls back", func() {
				accountID := insertAccount(ctx, pool)
				instruction := allocatedHold(accountID, "rollback-origin", "rollback-reference")
				sut := repository.NewHoldRepository(pool)
				rollbackTx, err := pool.Begin(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(sut.Save(database.WithTx(ctx, rollbackTx), instruction)).To(Succeed())
				Expect(rollbackTx.Rollback(ctx)).To(Succeed())
				var count int
				err = pool.QueryRow(ctx, `SELECT count(*) FROM holds WHERE origin=$1 AND reference=$2`, instruction.Origin, instruction.Reference).Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeZero())
			})

			It("should enforce release idempotency and preserve its fingerprint", func() {
				accountID := insertAccountV7(ctx, tx)
				instruction := allocatedHold(accountID, "release-origin", "release-reference")
				sut := repository.NewHoldRepository(pool)
				txContext := database.WithTx(ctx, tx)
				Expect(sut.Save(txContext, instruction)).To(Succeed())
				release := domainhold.Release{ID: newIDV7(), HoldID: instruction.ID, IdempotencyKey: "release-" + uuid.NewString(),
					Amount: amount(100, money.BRL), AuthorizationReference: "authorization-1", RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
				Expect(sut.SaveRelease(txContext, release)).To(Succeed())
				loaded, err := sut.FindRelease(txContext, release.IdempotencyKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(loaded).NotTo(BeNil())
				Expect(loaded.RequestFingerprint).To(Equal(release.RequestFingerprint))
				Expect(sut.SaveRelease(txContext, release)).To(MatchError(ContainSubstring("conflicts with existing release")))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: HOLD OUTBOX ATOMICITY", func() {
	It("should roll back hold and history when the outbox event cannot be saved", func() {
		accountID := insertAccountV7(ctx, tx)
		instruction := allocatedHold(accountID, "atomic-origin", "atomic-reference")
		holdRepo := repository.NewHoldRepository(pool)
		outboxRepo := repository.NewOutBoxRepository(pool)
		txContext := database.WithTx(ctx, tx)
		Expect(holdRepo.Save(txContext, instruction)).To(Succeed())
		history := domainhold.History{ID: newIDV7(), HoldID: instruction.ID, EventType: "DECISION_RECORDED",
			Requested: instruction.Requested, AllocatedDelta: instruction.Allocated,
			Pending: instruction.Pending, ReleasedDelta: amount(0, money.BRL), CreatedAt: time.Now().UTC()}
		Expect(holdRepo.SaveHistory(txContext, history)).To(Succeed())
		invalidID, err := entity.NewIDV7()
		Expect(err).NotTo(HaveOccurred())
		invalidOutbox := &outbox.Outbox{ID: invalidID, AggregateID: instruction.ID,
			AggregateType: outbox.Hold, EventType: outbox.HoldEvaluated,
			Status: "INVALID", CreatedAt: time.Now().UTC()}
		Expect(outboxRepo.Save(txContext, invalidOutbox)).To(HaveOccurred())
		Expect(tx.Rollback(ctx)).To(Succeed())
		tx = nil

		var holds, histories int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM holds WHERE origin=$1 AND reference=$2`, instruction.Origin, instruction.Reference).Scan(&holds)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1`, instruction.ID).Scan(&histories)).To(Succeed())
		Expect(holds).To(BeZero())
		Expect(histories).To(BeZero())
	})
})

func allocatedHold(accountID, origin, reference string) *domainhold.Instruction {
	return &domainhold.Instruction{
		ID: newIDV7(), AccountID: accountID, Currency: string(money.BRL), Origin: origin,
		Reference: reference, Type: domainhold.Judicial, Requested: amount(900, money.BRL),
		Allocated: amount(900, money.BRL), Pending: amount(0, money.BRL), Released: amount(0, money.BRL),
		Status: domainhold.StatusAllocated, PolicyVersion: "policy-v1",
		RequestFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
}

func amount(value int64, currency money.Currency) money.Money {
	result, err := money.NewMoney(value, currency)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return result
}
