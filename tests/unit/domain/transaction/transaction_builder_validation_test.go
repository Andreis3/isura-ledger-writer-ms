//go:build unit
// +build unit

package transaction_test

import (
	"strings"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	transferDebitAccount  = "e4e5e6e7-e8e9-410e-a11e-e12e13e14e15"
	transferCreditAccount = "f4f5f6f7-f8f9-410f-a11f-f12f13f14f15"
)

func buildEntry(direction transaction.Direction, accountID string, amount money.Money) *transaction.Entry {
	entry, err := transaction.NewEntryBuilder().
		WithID().
		WithAccountExternalID(accountID).
		WithDirection(direction).
		WithAmount(amount).
		Build()
	Expect(err).NotTo(HaveOccurred())
	return entry
}

var _ = Describe("INTERNAL :: DOMAIN :: TRANSACTION :: TRANSACTION", func() {
	Describe("#TransactionBuilder validation", func() {
		Context("error cases", func() {
			It("should reject an invalid supplied transaction ID", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID("bad-id").
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject an empty or overlong idempotency key", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				for _, key := range []string{"", strings.Repeat("x", 51)} {
					_, err = transaction.NewTransactionBuilder().
						WithID().
						WithIdempotencyKey(key).
						WithAmount(amount).
						WithOperation(transaction.OperationDeposit).
						Build()
					Expect(err).To(HaveOccurred())
				}
			})

			It("should reject an invalid supplied status", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithStatus("UNKNOWN").
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject an invalid operation", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation("UNKNOWN").
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject a fingerprint with an invalid length", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithFingerprint("not-a-sha256-digest").
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject a future update time", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationDeposit).
					WithUpdatedAt(time.Now().AddDate(100, 0, 0)).
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject a transfer without two entries", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationTransfer).
					Build()

				Expect(err).To(HaveOccurred())
			})

			It("should reject a transfer with equal directions", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entries := []*transaction.Entry{
					buildEntry(transaction.Credit, transferCreditAccount, amount),
					buildEntry(transaction.Credit, transferDebitAccount, amount),
				}

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationTransfer).
					WithEntries(entries).
					Build()
				Expect(err).To(HaveOccurred())
			})

			It("should reject a transfer with different amounts", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				largerAmount, err := money.NewMoney(200, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entries := []*transaction.Entry{
					buildEntry(transaction.Credit, transferCreditAccount, amount),
					buildEntry(transaction.Debit, transferDebitAccount, largerAmount),
				}

				_, err = transaction.NewTransactionBuilder().
					WithID().
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationTransfer).
					WithEntries(entries).
					Build()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("success cases", func() {
			It("should build a transfer with distinct debit and credit entries", func() {
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entries := []*transaction.Entry{
					buildEntry(transaction.Credit, transferCreditAccount, amount),
					buildEntry(transaction.Debit, transferDebitAccount, amount),
				}
				id, err := entity.NewIDV7()
				Expect(err).NotTo(HaveOccurred())

				built, err := transaction.NewTransactionBuilder().
					WithID(id.String()).
					WithIdempotencyKey("key").
					WithAmount(amount).
					WithOperation(transaction.OperationTransfer).
					WithStatus().
					WithUpdatedAt().
					WithEntries(entries).
					WithFingerprint(strings.Repeat("a", 64)).
					Build()

				Expect(err).NotTo(HaveOccurred())
				Expect(built.Status).To(Equal(transaction.Pending))
				Expect(built.Entries).To(HaveLen(2))
			})
		})
	})
})
