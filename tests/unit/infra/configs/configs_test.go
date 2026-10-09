//go:build unit

package configs_test

import (
	"errors"
	"os"
	"time"

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

				loaded, err := configs.LoadConfig()

				Expect(err).NotTo(HaveOccurred())
				Expect(loaded).NotTo(BeNil())
				Expect(loaded.Transaction.MaxEntries).To(Equal(application.DefaultMaxTransactionEntries))
			})

			It("should use the configured transaction entry limit from the environment", func() {
				GinkgoT().Setenv("TRANSACTION_MAX_ENTRIES", "23")

				loaded, err := configs.LoadConfig()

				Expect(err).NotTo(HaveOccurred())
				Expect(loaded).NotTo(BeNil())
				Expect(loaded.Transaction.MaxEntries).To(Equal(23))
			})

			It("should use the configured outbox shutdown timeout from the environment", func() {
				GinkgoT().Setenv("NATS_RELAY_SHUTDOWN_TIMEOUT", "3s")

				loaded, err := configs.LoadConfig()

				Expect(err).NotTo(HaveOccurred())
				Expect(loaded.Nats.Relay.ShutdownTimeout).To(Equal(3 * time.Second))
			})
		})

		Context("error cases", func() {
			It("should preserve the cause when the configuration file is malformed", func() {
				GinkgoT().Setenv("TRANSACTION_MAX_ENTRIES", "")
				Expect(os.WriteFile("config.json", []byte("{"), 0o600)).To(Succeed())

				loaded, err := configs.LoadConfig()

				Expect(loaded).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(errors.Unwrap(err)).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("read config file:"))
				Expect(err.Error()).To(ContainSubstring("unexpected end of JSON input"))
			})

			It("should preserve the cause when a configuration value cannot be unmarshaled", func() {
				GinkgoT().Setenv("TRANSACTION_MAX_ENTRIES", "")
				Expect(os.WriteFile("config.json", []byte(`{"transaction":{"max_entries":"invalid"}}`), 0o600)).To(Succeed())

				loaded, err := configs.LoadConfig()

				Expect(loaded).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(errors.Unwrap(err)).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("unmarshal config:"))
			})
		})
	})
})
