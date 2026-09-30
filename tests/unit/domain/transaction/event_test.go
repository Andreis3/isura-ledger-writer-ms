//go:build unit

package transaction_test

import (
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: EVENT", func() {
	Describe("#Build", func() {
		Context("success cases", func() {
			It("should copy metadata and preserve event values", func() {
				// Arrange (Given)
				now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
				metadata := map[string]string{"source": "test"}
				event := transaction.NewTransactionCreated().
					WithEventID("event").
					WithTransactionID("transaction").
					WithIdempotencyKey("key").
					WithDebitAccountID("debit").
					WithCreditAccountID("credit").
					WithAmount(1500).
					WithCurrency("BRL").
					WithStatus("COMPLETED").
					WithOccurredAt(now).
					WithMetadata(metadata).
					Build()

				// Act (When)
				metadata["source"] = "changed"

				// Assert (Then)
				Expect(event.Metadata["source"]).To(Equal("test"))
				Expect(event.OccurredAt).To(Equal(now))
				Expect(event.Amount).To(Equal(int64(1500)))
			})
		})
	})
})
