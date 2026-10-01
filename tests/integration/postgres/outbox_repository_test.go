//go:build integration

package postgres_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: OUTBOX REPOSITORY", func() {
	Describe("#Save, #ClaimPending, and #UpdateOutboxData", func() {
		Context("success cases", func() {
			It("should claim a pending event and persist its published state", func() {
				// Arrange (Given)
				_, err := tx.Exec(ctx, `DELETE FROM outbox_events`)
				Expect(err).NotTo(HaveOccurred())
				item := newOutbox("transaction-aggregate")
				repo := repository.NewOutBoxRepository(pool)
				txContext := database.WithTx(ctx, tx)
				Expect(repo.Save(txContext, item)).To(Succeed())

				// Act (When)
				claimed, err := repo.ClaimPending(txContext, 10, outbox.MaxAttempts, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				Expect(claimed).To(HaveLen(1))
				publishedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				err = repo.UpdateOutboxData(txContext, item.ID, outbox.UpdateOutboxData{
					Status: outbox.Success, Attempts: claimed[0].Attempts, LastAttemptAt: claimed[0].LastAttemptAt, PublishedAt: &publishedAt,
				})
				Expect(err).NotTo(HaveOccurred())

				// Assert (Then)
				Expect(claimed[0].ID).To(Equal(item.ID))
				Expect(claimed[0].Status).To(Equal(outbox.Failed))
				Expect(claimed[0].Attempts).To(Equal(1))
				published, err := repo.FindAll(txContext, outbox.Success, 10)
				Expect(err).NotTo(HaveOccurred())
				Expect(published).To(HaveLen(1))
				Expect(published[0].ID).To(Equal(item.ID))
				Expect(published[0].PublishedAt).NotTo(BeNil())
			})
		})
	})
})
