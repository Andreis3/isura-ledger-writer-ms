//go:build unit

package criteria_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: REPOSITORY :: CRITERIA", func() {
	Describe("#GetAccountCriteria", func() {
		Context("success cases", func() {
			It("should preserve the base query and limit results when no filters are set", func() {
				query, args := criteria.GetAccountCriteria("SELECT * FROM accounts WHERE 1=1", criteria.AccountCriteria{})
				Expect(query).To(Equal("SELECT * FROM accounts WHERE 1=1 LIMIT 1"))
				Expect(args).To(BeEmpty())
			})

			It("should append all filters in positional order", func() {
				query, args := criteria.GetAccountCriteria("SELECT * FROM accounts WHERE 1=1", criteria.AccountCriteria{
					ID: ptr("id"), AccountExternalID: ptr("external"), TaxID: ptr("tax"),
					AccountNumber: ptr("number"), Currency: ptr("BRL"), Type: ptr("checking"),
				})
				Expect(query).To(Equal("SELECT * FROM accounts WHERE 1=1 AND id = $1 AND account_external_id = $2 AND tax_id = $3 AND account_number = $4 AND currency = $5 AND type = $6 LIMIT 1"))
				Expect(args).To(Equal([]any{"id", "external", "tax", "number", "BRL", "checking"}))
			})

			It("should select the requested account lock mode", func() {
				locked, _ := criteria.GetAccountCriteria("SELECT * FROM accounts WHERE 1=1", criteria.AccountCriteria{HasForUpdate: true, HasForUpdateSkipLock: true})
				skipLocked, _ := criteria.GetAccountCriteria("SELECT * FROM accounts WHERE 1=1", criteria.AccountCriteria{HasForUpdateSkipLock: true})
				Expect(locked).To(HaveSuffix(" FOR UPDATE LIMIT 1"))
				Expect(skipLocked).To(HaveSuffix(" FOR UPDATE SKIP LOCKED LIMIT 1"))
			})
		})
	})

	Describe("#GetBalanceCriteria", func() {
		Context("success cases", func() {
			It("should append balance filters and limit results", func() {
				query, args := criteria.GetBalanceCriteria("SELECT * FROM balances WHERE 1=1", criteria.BalanceCriteria{ID: ptr("balance"), AccountID: ptr("account")})
				Expect(query).To(Equal("SELECT * FROM balances WHERE 1=1 AND id = $1 AND account_id = $2 LIMIT 1"))
				Expect(args).To(Equal([]any{"balance", "account"}))
			})

			It("should append skip locked when requested", func() {
				query, _ := criteria.GetBalanceCriteria("SELECT * FROM balances WHERE 1=1", criteria.BalanceCriteria{HasForUpdateSkipLock: true, WithEntries: true})
				Expect(query).To(HaveSuffix(" FOR UPDATE SKIP LOCKED LIMIT 1"))
			})
		})
	})

	Describe("#GetEntryCriteria", func() {
		Context("success cases", func() {
			It("should limit results when no transaction is specified", func() {
				query, args := criteria.GetEntryCriteria("SELECT * FROM entries WHERE 1=1", criteria.EntryCriteria{})
				Expect(query).To(Equal("SELECT * FROM entries WHERE 1=1 LIMIT 2"))
				Expect(args).To(BeEmpty())
			})

			It("should filter by transaction id", func() {
				query, args := criteria.GetEntryCriteria("SELECT * FROM entries WHERE 1=1", criteria.EntryCriteria{TransactionID: ptr("tx")})
				Expect(query).To(Equal("SELECT * FROM entries WHERE 1=1 AND transaction_id = $1 LIMIT 2"))
				Expect(args).To(Equal([]any{"tx"}))
			})
		})
	})

	Describe("#GetTransactionCriteria", func() {
		Context("success cases", func() {
			It("should apply transaction identifiers in positional order", func() {
				query, args := criteria.GetTransactionCriteria("SELECT * FROM transactions WHERE 1=1", transaction.TransactionCriteria{ID: ptr("id"), IdempotencyKey: ptr("key")})
				Expect(query).To(Equal("SELECT * FROM transactions WHERE 1=1 AND id = $1 AND idempotency_key = $2 LIMIT 1"))
				Expect(args).To(Equal([]any{"id", "key"}))
			})

			It("should add the result limit when no filters are set", func() {
				query, args := criteria.GetTransactionCriteria("SELECT * FROM transactions WHERE 1=1", transaction.TransactionCriteria{})
				Expect(query).To(Equal("SELECT * FROM transactions WHERE 1=1 LIMIT 1"))
				Expect(args).To(BeEmpty())
			})
		})
	})
})

func ptr(value string) *string { return &value }
