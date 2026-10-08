//go:build unit

package configs_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: CONFIGS :: CONFIGS", func() {
	Describe("#LoadConfig", func() {
		Context("success cases", func() {
			It("should use the default transaction entry limit when no override is configured", func() {
				GinkgoT().Setenv("TRANSACTION_MAX_ENTRIES", "")

				loaded := configs.LoadConfig()

				Expect(loaded).NotTo(BeNil())
				Expect(loaded.Transaction.MaxEntries).To(Equal(application.DefaultMaxTransactionEntries))
			})

			It("should use the configured transaction entry limit from the environment", func() {
				GinkgoT().Setenv("TRANSACTION_MAX_ENTRIES", "23")

				loaded := configs.LoadConfig()

				Expect(loaded).NotTo(BeNil())
				Expect(loaded.Transaction.MaxEntries).To(Equal(23))
			})
		})
	})
})
