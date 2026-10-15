//go:build unit

package model_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/model"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: BALANCE", func() {
	Describe("#ToBalanceModel", func() {
		Context("success cases", func() {
			It("should preserve balance identity, account, amount, and timestamps", func() {
				// Arrange.
				created := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
				updated := created.Add(time.Hour)
				entity, err := balance.NewBalanceBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa0674469b7").
					WithAccountID("019ff448-c43d-70d3-83c7-dfa0674469b8").
					WithAmount(1250, money.BRL).
					WithCreatedAt(created).
					WithUpdatedAt(updated).
					Build()
				Expect(err).NotTo(HaveOccurred())

				// Act.
				persisted := model.ToBalanceModel(entity)
				restored, err := model.ToBalanceDomain(persisted)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.TableName()).To(Equal("balances"))
				Expect(restored.ID().String()).To(Equal(entity.ID().String()))
				Expect(restored.AccountID()).To(Equal(entity.AccountID()))
				Expect(restored.Amount().Amount()).To(Equal(int64(1250)))
				Expect(restored.Amount().Currency()).To(Equal(money.BRL))
				Expect(restored.CreatedAT()).To(Equal(created))
				Expect(restored.UpdatedAT()).To(Equal(updated))
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: MODEL :: OUTBOX", func() {
	Describe("#ToOutboxModel", func() {
		Context("success cases", func() {
			It("should preserve outbox data when optional timestamps are absent", func() {
				// Arrange.
				entity, err := outbox.NewOutbox("019ff448-c43d-70d3-83c7-dfa0674469b8", []byte(`{"event":"created"}`))
				Expect(err).NotTo(HaveOccurred())

				// Act.
				persisted := model.ToOutboxModel(entity)
				restored, err := model.ToOutboxDomain(persisted)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.LastAttemptAt.Valid).To(BeFalse())
				Expect(persisted.PublishedAt.Valid).To(BeFalse())
				Expect(restored.ID.String()).To(Equal(entity.ID.String()))
				Expect(restored.Payload).To(Equal(entity.Payload))
				Expect(restored.LastAttemptAt).To(BeNil())
				Expect(restored.PublishedAt).To(BeNil())
			})
		})

		Context("success cases", func() {
			It("should restore optional timestamps when they are present", func() {
				// Arrange.
				lastAttempt := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
				published := lastAttempt.Add(time.Minute)
				created := lastAttempt.Add(-time.Hour)
				entity, err := outbox.NewOutboxBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa0674469b9").
					WithAggregateID("019ff448-c43d-70d3-83c7-dfa0674469b8").
					WithAggregateType(outbox.Transaction).
					WithEventType(outbox.TransactionCreated).
					WithStatus(outbox.Success).
					WithAttempts(2).
					WithLastAttemptAt(lastAttempt).
					WithPublishedAt(published).
					WithCreatedAt(created).
					WithPayload([]byte(`{"event":"created"}`)).
					Build()
				Expect(err).NotTo(HaveOccurred())

				// Act.
				persisted := model.ToOutboxModel(entity)
				restored, err := model.ToOutboxDomain(persisted)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(restored.LastAttemptAt).To(Equal(&lastAttempt))
				Expect(restored.PublishedAt).To(Equal(&published))
				Expect(database.ToTimePtr(persisted.LastAttemptAt)).To(Equal(&lastAttempt))
			})
		})
	})

	Describe("#ToTimestamptz", func() {
		Context("success cases", func() {
			It("should preserve a timestamp and represent nil as invalid", func() {
				// Arrange.
				at := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)

				// Act.
				present := database.ToTimestamptz(&at)
				missing := database.ToTimestamptz(nil)

				// Assert.
				Expect(present).To(Equal(pgtype.Timestamptz{Time: at, Valid: true}))
				Expect(missing.Valid).To(BeFalse())
				Expect(database.ToTimePtr(missing)).To(BeNil())
			})
		})
	})
})
