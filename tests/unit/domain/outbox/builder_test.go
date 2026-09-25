//go:build unit

package outbox_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("outbox builder", func() {
	It("builds with defaults and explicit fields", func() {
		createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		lastAttempt := createdAt.Add(time.Minute)
		publishedAt := createdAt.Add(2 * time.Minute)

		result, err := outbox.NewOutboxBuilder().
			WithID().
			WithAggregateID("transaction-1").
			WithAggregateType(outbox.Transaction).
			WithEventType(outbox.TransactionCreated).
			WithStatus(outbox.Pending).
			WithAttempts(2).
			WithLastAttemptAt(lastAttempt).
			WithCreatedAt(createdAt).
			WithPublishedAt(publishedAt).
			WithPayload([]byte("payload")).
			Build()

		Expect(err).NotTo(HaveOccurred())
		Expect(result.AggregateID).To(Equal("transaction-1"))
		Expect(result.Attempts).To(Equal(2))
		Expect(result.CreatedAt).To(Equal(createdAt))
		Expect(*result.LastAttemptAt).To(Equal(lastAttempt))
		Expect(*result.PublishedAt).To(Equal(publishedAt))
		Expect(result.Payload).To(Equal([]byte("payload")))
	})

	It("rejects invalid fields", func() {
		result, err := outbox.NewOutboxBuilder().
			WithID("invalid").
			WithAggregateID("").
			WithAggregateType(outbox.AggregateType("invalid")).
			WithEventType(outbox.EventType("invalid")).
			WithStatus(outbox.StatusOutbox("invalid")).
			Build()

		Expect(result).To(BeNil())
		Expect(err).To(HaveOccurred())
	})
})
