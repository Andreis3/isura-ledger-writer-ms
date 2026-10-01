//go:build unit

package event_test

import (
	"encoding/json"

	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: EVENT :: EVENT", func() {
	Describe("#NewCreateBalanceEvent", func() {
		Context("success cases", func() {
			It("should expose the balance event subject and payload", func() {
				// Arrange (Given)
				created := event.NewCreateBalanceEvent("account-1", "BRL")

				// Act (When)
				Expect(created.SubjectName()).To(Equal("ledger.writer.event"))
				payload, err := created.Payload()
				Expect(err).NotTo(HaveOccurred())

				var decoded event.CreateBalance
				Expect(json.Unmarshal(payload, &decoded)).To(Succeed())
				Expect(decoded).To(Equal(*created))
			})
		})
	})

	Describe("#NewDeadLetterEvent", func() {
		Context("success cases", func() {
			It("should append the dead-letter suffix and preserve the payload", func() {
				// Arrange (Given)
				payload := []byte(`{"id":"event-1"}`)
				deadLetter := event.NewDeadLetterEvent("ledger.transaction.created", payload)

				// Act (When)
				Expect(deadLetter.SubjectName()).To(Equal("ledger.transaction.created.dlq"))
				actual, err := deadLetter.Payload()
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(Equal(payload))
			})
		})
	})
})
