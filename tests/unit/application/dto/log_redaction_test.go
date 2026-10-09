//go:build unit

package dto_test

import (
 "log/slog"
 "strings"

 "github.com/andreis3/isura-ledger-ms/internal/application/dto"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: DTO :: TRANSACTION LOG REDACTION", func() {
 Describe("#LogValue", func() {
  Context("success cases", func() {
   It("should conceal sensitive identifiers and metadata in rendered slog records", func() {
    key := "sensitive-idempotency-key-123"
    debit := "d290f1ee-6c54-4b01-90e6-d701748f0851"
    credit := "a290f1ee-6c54-4b01-90e6-d701748f0852"
    value := dto.CreateTransactionInput{
     IdempotencyKey: &key,
     DebitAccountID: &debit,
     CreditAccountID: &credit,
     Metadata: map[string]string{"personal_data":"sensitive-customer-value"},
    }
    // Act.
    var log strings.Builder
    logger := slog.New(slog.NewJSONHandler(&log, nil))
    logger.Info("transaction input", "input", value)
    output := log.String()

    // Assert.
    Expect(output).To(ContainSubstring("[REDACTED]"))
    Expect(output).To(ContainSubstring("****0851"))
    Expect(output).To(ContainSubstring("****0852"))
    Expect(output).NotTo(ContainSubstring(key))
    Expect(output).NotTo(ContainSubstring(debit))
    Expect(output).NotTo(ContainSubstring(credit))
    Expect(output).NotTo(ContainSubstring("sensitive-customer-value"))
   })
   It("should completely redact very short account identifiers", func() {
    debit := "1234"
    credit := "a"
    input := dto.CreateTransactionInput{DebitAccountID: &debit, CreditAccountID: &credit}
    fields := logFields(input.LogValue())
    Expect(fields["debit_account_id"].String()).To(Equal("[REDACTED]"))
    Expect(fields["credit_account_id"].String()).To(Equal("[REDACTED]"))
   })
  })
 })
})
