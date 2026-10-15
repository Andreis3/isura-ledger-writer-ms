//go:build unit

package transaction_test

import (
	"encoding/json"
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
				entries := []transaction.TransactionEntryCreated{{
					Position: 0, AccountID: "account", Direction: transaction.Debit, Amount: 1500, Currency: "BRL",
				}}
				movements := []transaction.TransactionMovementCreated{{MovementID: "movement", Type: "TRANSFER", Description: "transfer", EntryIDs: []string{"entry"}}}
				event := transaction.NewTransactionCreated().
					WithEventVersion(1).
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
					WithEntries(entries).
					WithMovements(movements).
					Build()

				// Act (When)
				metadata["source"] = "changed"
				entries[0].AccountID = "changed"
				movements[0].EntryIDs[0] = "changed"

				// Assert (Then)
				Expect(event.Metadata["source"]).To(Equal("test"))
				Expect(event.OccurredAt).To(Equal(now))
				Expect(event.Amount).To(Equal(int64(1500)))
				Expect(event.EventVersion).To(Equal(1))
				Expect(event.Movements[0].EntryIDs).To(Equal([]string{"entry"}))
				Expect(event.Entries).To(Equal([]transaction.TransactionEntryCreated{{
					Position: 0, AccountID: "account", Direction: transaction.Debit, Amount: 1500, Currency: "BRL",
				}}))
			})
		})

		It("should accept a legacy event without version or movements", func() {
			// Arrange (Given)
			payload := []byte(`{"event_id":"legacy","transaction_id":"transaction","entries":[]}`)

			// Act (When)
			var event transaction.TransactionCreated
			err := json.Unmarshal(payload, &event)

			// Assert (Then)
			Expect(err).NotTo(HaveOccurred())
			Expect(event.EventVersion).To(BeZero())
			Expect(event.Movements).To(BeNil())
		})
	})
})
