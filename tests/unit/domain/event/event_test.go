//go:build unit

package event_test

import (
	"encoding/json"

	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("domain events", func() {
	It("exposes the balance event subject and payload", func() {
		created := event.NewCreateBalanceEvent("account-1", "BRL")

		Expect(created.SubjectName()).To(Equal("ledger.writer.event"))
		payload, err := created.Payload()
		Expect(err).NotTo(HaveOccurred())

		var decoded event.CreateBalance
		Expect(json.Unmarshal(payload, &decoded)).To(Succeed())
		Expect(decoded).To(Equal(*created))
	})

	It("appends the dead-letter suffix and preserves the payload", func() {
		payload := []byte(`{"id":"event-1"}`)
		deadLetter := event.NewDeadLetterEvent("ledger.transaction.created", payload)

		Expect(deadLetter.SubjectName()).To(Equal("ledger.transaction.created.dlq"))
		actual, err := deadLetter.Payload()
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(payload))
	})
})
