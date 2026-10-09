//go:build unit

package dto_test

import (
	"log/slog"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func logFields(value slog.Value) map[string]slog.Value {
	result := make(map[string]slog.Value)
	for _, attr := range value.Group() {
		result[attr.Key] = attr.Value
	}
	return result
}

var _ = Describe("INTERNAL :: APPLICATION :: DTO :: LOG VALUE", func() {
	Describe("#CreateAccountInput.LogValue", func() {
		Context("success cases", func() {
			It("should mask tax ID and account number while retaining non-sensitive fields", func() {
				input := dto.CreateAccountInput{
					AccountExternalID: "d589965c-1622-4329-98f9-f13354a2e4dc",
					AccountNumber:     "1234567890123456",
					TaxID:             "52998224725",
					AccountType:       "ASSET",
					BalancePolicy:     "BALANCE_NON_NEGATIVE",
					Currency:          "BRL",
				}
				fields := logFields(input.LogValue())
				Expect(fields).To(HaveLen(6))
				Expect(fields["account_number"].String()).NotTo(Equal(input.AccountNumber))
				Expect(fields["account_number"].String()).NotTo(ContainSubstring(input.AccountNumber))
				Expect(fields["tax_id"].String()).NotTo(Equal(input.TaxID))
				Expect(fields["tax_id"].String()).NotTo(ContainSubstring(input.TaxID))
				Expect(fields["currency"].String()).To(Equal("BRL"))
				Expect(fields["account_type"].String()).To(Equal("ASSET"))
			})
		})
	})
	Describe("#CreateTransactionInput.LogValue", func() {
		Context("success cases", func() {
			It("should omit metadata values and handle absent optional fields", func() {
				empty := logFields((dto.CreateTransactionInput{}).LogValue())
				Expect(empty["idempotency_key"].String()).To(BeEmpty())
				Expect(empty["debit_account_id"].String()).To(BeEmpty())
				Expect(empty["credit_account_id"].String()).To(BeEmpty())
				Expect(empty["amount"].Int64()).To(BeZero())
				Expect(empty["currency"].String()).To(BeEmpty())
				key := "unique-key"
				debit := "debit-account"
				credit := "credit-account"
				currency := "BRL"
				amount := int64(1500)
				input := dto.CreateTransactionInput{
					IdempotencyKey: &key, DebitAccountID: &debit, CreditAccountID: &credit,
					Currency: &currency, Amount: &amount, Metadata: map[string]string{"secret": "do-not-log-me"},
				}
				fields := logFields(input.LogValue())
				Expect(fields["amount"].Int64()).To(Equal(amount))
				Expect(fields["currency"].String()).To(Equal(currency))
				Expect(fields["metadata_entries"].Int64()).To(Equal(int64(1)))
				Expect(fields).NotTo(HaveKey("metadata"))
				for _, field := range fields {
					Expect(field.String()).NotTo(ContainSubstring("do-not-log-me"))
				}
				// Idempotency keys are never written to logs; only short account suffixes remain.
				Expect(fields["idempotency_key"].String()).To(Equal("[REDACTED]"))
				Expect(fields["debit_account_id"].String()).To(Equal("****ount"))
				Expect(fields["credit_account_id"].String()).To(Equal("****ount"))
				Expect(fields["debit_account_id"].String()).NotTo(Equal(debit))
				Expect(fields["credit_account_id"].String()).NotTo(Equal(credit))
			})
		})
	})
})
