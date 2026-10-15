//go:build unit
// +build unit

package dto_test

import (
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
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

	It("rejects invalid transfer amount and currency", func() {
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
		} {
			entity, err := input.CreateTransactionFacade()
			Expect(entity).To(BeNil())
			Expect(err).To(HaveOccurred())
		}
	})

})

var _ = Describe("INTERNAL :: APPLICATION :: DTO :: CREATE TRANSACTION", func() {
	Describe("#Fingerprint", func() {
		Context("success cases", func() {
			It("should include movement type, description, and entry positions in the fingerprint", func() {
				base := multiEntryInput()
				base.Movements = []dto.MovementInput{{Type: "TRANSFER", Description: "main transfer", EntryPositions: []int{0, 1}}}
				original, err := base.Fingerprint()
				Expect(err).NotTo(HaveOccurred())

				typeChanged := base
				typeChanged.Movements = []dto.MovementInput{{Type: "REVERSAL", Description: "main transfer", EntryPositions: []int{0, 1}}}
				descriptionChanged := base
				descriptionChanged.Movements = []dto.MovementInput{{Type: "TRANSFER", Description: "other transfer", EntryPositions: []int{0, 1}}}
				positionsChanged := base
				positionsChanged.Movements = []dto.MovementInput{{Type: "TRANSFER", Description: "main transfer", EntryPositions: []int{1, 0}}}

				for _, changed := range []dto.CreateTransactionInput{typeChanged, descriptionChanged, positionsChanged} {
					fingerprint, fingerprintErr := changed.Fingerprint()
					Expect(fingerprintErr).NotTo(HaveOccurred())
					Expect(fingerprint).NotTo(Equal(original))
				}
			})

			It("should preserve movement and referenced entry order in the fingerprint", func() {
				first := multiEntryInput()
				first.Movements = []dto.MovementInput{
					{Type: "TRANSFER", Description: "first", EntryPositions: []int{0}},
					{Type: "TRANSFER", Description: "second", EntryPositions: []int{1}},
				}
				second := first
				second.Movements = append([]dto.MovementInput(nil), first.Movements...)
				second.Movements[0], second.Movements[1] = second.Movements[1], second.Movements[0]

				firstFingerprint, err := first.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				secondFingerprint, err := second.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				Expect(secondFingerprint).NotTo(Equal(firstFingerprint))
			})

			It("should create the same fingerprint for identical ordered entries", func() {
				input := multiEntryInput()
				first, err := input.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				second, err := input.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				Expect(second).To(Equal(first))
			})

			It("should normalize legacy fields to the same canonical entries", func() {
				input := dto.CreateTransactionInput{
					IdempotencyKey:  stringPointer("legacy-1"),
					DebitAccountID:  stringPointer("d290f1ee-6c54-4b01-90e6-d701748f0851"),
					CreditAccountID: stringPointer("a290f1ee-6c54-4b01-90e6-d701748f0852"),
					Amount:          intPointer(150000), Currency: stringPointer("BRL"), Operation: stringPointer("TRANSFER"),
				}
				entries, err := input.CreateTransactionFacade()
				Expect(err).NotTo(HaveOccurred())
				Expect(entries.Entries).To(HaveLen(2))
				Expect(entries.Entries[0].Direction).To(Equal(transaction.Credit))
				Expect(entries.Entries[1].Direction).To(Equal(transaction.Debit))

				legacyFingerprint, err := input.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				explicitFingerprint, err := (dto.CreateTransactionInput{
					Amount:    intPointer(150000),
					Operation: stringPointer("TRANSFER"),
					Entries: []dto.EntryInput{
						{AccountID: "a290f1ee-6c54-4b01-90e6-d701748f0852", Direction: "CREDIT", Amount: 150000, Currency: "BRL"},
						{AccountID: "d290f1ee-6c54-4b01-90e6-d701748f0851", Direction: "DEBIT", Amount: 150000, Currency: "BRL"},
					},
				}).Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				Expect(legacyFingerprint).To(Equal(explicitFingerprint))
			})
		})

		Context("error cases", func() {
			It("should change the fingerprint when entry order or content changes", func() {
				first := multiEntryInput()
				original, err := first.Fingerprint()
				Expect(err).NotTo(HaveOccurred())

				reordered := multiEntryInput()
				reordered.Entries[0], reordered.Entries[1] = reordered.Entries[1], reordered.Entries[0]
				reorderedFingerprint, err := reordered.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				Expect(reorderedFingerprint).NotTo(Equal(original))

				changed := multiEntryInput()
				changed.Entries[0].Amount++
				changedFingerprint, err := changed.Fingerprint()
				Expect(err).NotTo(HaveOccurred())
				Expect(changedFingerprint).NotTo(Equal(original))
			})

			It("should reject a request mixing entries and legacy fields", func() {
				input := multiEntryInput()
				input.DebitAccountID = stringPointer("d290f1ee-6c54-4b01-90e6-d701748f0851")
				_, err := input.Fingerprint()
				Expect(err).To(HaveOccurred())
			})

			It("should reject an explicitly empty entries collection", func() {
				input := dto.CreateTransactionInput{Entries: []dto.EntryInput{}}
				_, err := input.Fingerprint()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("#CreateTransactionFacade", func() {
		Context("success cases", func() {
			It("should resolve movement positions to the corresponding transaction entries", func() {
				input := multiEntryInput()
				input.IdempotencyKey = stringPointer("movement-dto-1")
				input.Movements = []dto.MovementInput{{Type: "TRANSFER", Description: "main transfer", EntryPositions: []int{1, 0}}}

				entity, err := input.CreateTransactionFacade()

				Expect(err).NotTo(HaveOccurred())
				Expect(entity.Movements).To(HaveLen(1))
				Expect(entity.Movements[0].Type).To(Equal("TRANSFER"))
				Expect(entity.Movements[0].Description).To(Equal("main transfer"))
				Expect(entity.Movements[0].Entries).To(HaveLen(2))
				Expect(entity.Movements[0].Entries[0]).To(BeIdenticalTo(entity.Entries[1]))
				Expect(entity.Movements[0].Entries[1]).To(BeIdenticalTo(entity.Entries[0]))
			})
		})

		Context("error cases", func() {
			It("should reject movement positions outside the entries collection without echoing the description", func() {
				input := multiEntryInput()
				input.Movements = []dto.MovementInput{{Type: "TRANSFER", Description: "sensitive description", EntryPositions: []int{2}}}

				entity, err := input.CreateTransactionFacade()

				Expect(entity).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).NotTo(ContainSubstring("sensitive description"))
			})
			It("should build every requested entry in input order", func() {
				input := multiEntryInput()
				input.IdempotencyKey = stringPointer("multi-1")
				input.Entries = append(input.Entries, dto.EntryInput{
					AccountID: "b290f1ee-6c54-4b01-90e6-d701748f0853", Direction: "CREDIT", Amount: 25, Currency: "BRL",
				})
				input.Entries[0].Amount = 125
				input.Amount = intPointer(125)

				entity, err := input.CreateTransactionFacade()

				Expect(err).NotTo(HaveOccurred())
				Expect(entity.Entries).To(HaveLen(3))
				Expect(entity.Entries[0].Direction).To(Equal(transaction.Debit))
				Expect(entity.Entries[1].Direction).To(Equal(transaction.Credit))
				Expect(entity.Entries[2].AccountExternalID).To(Equal(input.Entries[2].AccountID))
				Expect(entity.Amount.Amount()).To(Equal(int64(125)))
			})

			It("should reject entries whose debit total differs from the root amount", func() {
				// Arrange.
				input := multiEntryInput()
				input.Amount = intPointer(101)

				// Act.
				entity, err := input.CreateTransactionFacade()

				// Assert.
				Expect(entity).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(transaction.ErrTransactionAmountMismatch.Error()))
			})

			It("should accept debit and credit entries for the same account", func() {
				accountID := "d290f1ee-6c54-4b01-90e6-d701748f0851"
				input := dto.CreateTransactionInput{
					IdempotencyKey:  stringPointer("same-account-transfer"),
					DebitAccountID:  stringPointer(accountID),
					CreditAccountID: stringPointer(accountID),
					Amount:          intPointer(100),
					Currency:        stringPointer("BRL"),
					Operation:       stringPointer("TRANSFER"),
				}

				entity, err := input.CreateTransactionFacade()

				Expect(err).NotTo(HaveOccurred())
				Expect(entity.Entries).To(HaveLen(2))
			})
		})
	})
})

func multiEntryInput() dto.CreateTransactionInput {
	return dto.CreateTransactionInput{
		Amount:    intPointer(100),
		Operation: stringPointer("TRANSFER"),
		Entries: []dto.EntryInput{
			{AccountID: "d290f1ee-6c54-4b01-90e6-d701748f0851", Direction: "DEBIT", Amount: 100, Currency: "BRL", Metadata: map[string]string{"leg": "source"}},
			{AccountID: "a290f1ee-6c54-4b01-90e6-d701748f0852", Direction: "CREDIT", Amount: 100, Currency: "BRL", Metadata: map[string]string{"leg": "destination"}},
		},
	}
}
