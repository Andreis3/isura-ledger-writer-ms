//go:build unit
// +build unit

package transaction_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: ENTRY", func() {
	Describe("#SetSequenceNumber", func() {
		Context("success cases", func() {
			It("should store the assigned account sequence", func() {
				entry := &transaction.Entry{}

				entry.SetSequenceNumber(42)

				Expect(entry.SequenceNumber).To(Equal(int64(42)))
			})
		})
	})

	Describe("#SetRunningBalance", func() {
		Context("success cases", func() {
			It("should store the balance after the entry is applied", func() {
				entry := &transaction.Entry{}

				entry.SetRunningBalance(1250)

				Expect(entry.RunningBalance).To(Equal(int64(1250)))
			})
		})
	})
})
