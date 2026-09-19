//go:build unit
// +build unit

package dto_test

import (
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func stringPointer(value string) *string { return &value }
func intPointer(value int64) *int64      { return &value }

var _ = Describe("CreateTransactionInput", func() {
	It("calculates the same fingerprint regardless of metadata map insertion order", func() {
		first := dto.CreateTransactionInput{
			DebitAccountID:  stringPointer("d290f1ee-6c54-4b01-90e6-d701748f0851"),
			CreditAccountID: stringPointer("a290f1ee-6c54-4b01-90e6-d701748f0852"),
			Amount:          intPointer(150000),
			Currency:        stringPointer("BRL"),
			Operation:       stringPointer("TRANSFER"),
			Metadata:        map[string]string{"request_id": "req-123", "source": "payments-ms"},
		}
		second := first
		second.Metadata = map[string]string{"source": "payments-ms", "request_id": "req-123"}

		firstFingerprint, err := first.Fingerprint()
		Expect(err).NotTo(HaveOccurred())
		secondFingerprint, err := second.Fingerprint()
		Expect(err).NotTo(HaveOccurred())
		Expect(firstFingerprint).To(HaveLen(64))
		Expect(secondFingerprint).To(Equal(firstFingerprint))
	})

	It("does not include the idempotency key in the fingerprint", func() {
		first := dto.CreateTransactionInput{IdempotencyKey: stringPointer("first")}
		second := first
		second.IdempotencyKey = stringPointer("second")

		firstFingerprint, err := first.Fingerprint()
		Expect(err).NotTo(HaveOccurred())
		secondFingerprint, err := second.Fingerprint()
		Expect(err).NotTo(HaveOccurred())
		Expect(secondFingerprint).To(Equal(firstFingerprint))
	})

	It("rejects metadata over the configured limit", func() {
		input := dto.CreateTransactionInput{Metadata: make(map[string]string, dto.MaxMetadataEntries+1)}
		for index := 0; index <= dto.MaxMetadataEntries; index++ {
			input.Metadata[string(rune('a'+index))] = "value"
		}

		_, err := input.Fingerprint()
		Expect(err).To(HaveOccurred())
		var domainError *fault.DomainError
		Expect(errors.As(err, &domainError)).To(BeTrue())
		Expect(domainError.Code).To(Equal(fault.CodeInvalidEntity))
	})

	It("builds a transfer with two opposite entries and carries metadata", func() {
		input := dto.CreateTransactionInput{
			IdempotencyKey:  stringPointer("transfer-1"),
			DebitAccountID:  stringPointer("d290f1ee-6c54-4b01-90e6-d701748f0851"),
			CreditAccountID: stringPointer("a290f1ee-6c54-4b01-90e6-d701748f0852"),
			Amount:          intPointer(150000),
			Currency:        stringPointer("BRL"),
			Operation:       stringPointer("TRANSFER"),
			Metadata:        map[string]string{"source": "payments-ms"},
		}

		entity, err := input.CreateTransactionFacade()
		Expect(err).NotTo(HaveOccurred())
		Expect(entity.Entries).To(HaveLen(2))
		Expect(entity.Entries[0].Amount).To(Equal(entity.Entries[1].Amount))
		Expect(entity.Entries[0].Direction).NotTo(Equal(entity.Entries[1].Direction))
		Expect(entity.Metadata).To(Equal(input.Metadata))
		Expect(entity.Fingerprint).To(HaveLen(64))
	})

	It("rejects invalid transfer amount, currency and equal accounts", func() {
		valid := dto.CreateTransactionInput{
			IdempotencyKey:  stringPointer("transfer-1"),
			DebitAccountID:  stringPointer("d290f1ee-6c54-4b01-90e6-d701748f0851"),
			CreditAccountID: stringPointer("a290f1ee-6c54-4b01-90e6-d701748f0852"),
			Amount:          intPointer(1),
			Currency:        stringPointer("BRL"),
			Operation:       stringPointer("TRANSFER"),
		}

		for _, input := range []dto.CreateTransactionInput{
			func() dto.CreateTransactionInput {
				copy := valid
				copy.Amount = intPointer(0)
				return copy
			}(),
			func() dto.CreateTransactionInput {
				copy := valid
				copy.Currency = stringPointer("JPY")
				return copy
			}(),
			func() dto.CreateTransactionInput {
				copy := valid
				copy.CreditAccountID = copy.DebitAccountID
				return copy
			}(),
		} {
			entity, err := input.CreateTransactionFacade()
			Expect(entity).To(BeNil())
			Expect(err).To(HaveOccurred())
		}
	})
})
