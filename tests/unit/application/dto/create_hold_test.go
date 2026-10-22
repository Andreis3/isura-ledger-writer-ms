//go:build unit

package dto_test

import (
	"errors"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: DTO :: CREATE HOLD", func() {
	Describe("#CreateHoldFacade", func() {
		Context("success cases", func() {
			It("should create a pending instruction with a stable fingerprint", func() {
				amount, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: amount}

				instruction, err := input.CreateHoldFacade()

				Expect(err).NotTo(HaveOccurred())
				Expect(instruction.Status).To(Equal(hold.StatusPendingReview))
				Expect(instruction.Allocated.Amount()).To(BeZero())
				Expect(instruction.Pending).To(Equal(amount))
				Expect(instruction.RequestFingerprint).To(HaveLen(64))
			})
		})
		Context("error cases", func() {
			It("should reject invalid instruction identity and type fields", func() {
				amount, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				base := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "issuer", Reference: "ref-1", Type: hold.Judicial, Requested: amount}

				missingAccount := base
				missingAccount.AccountID = ""
				_, err = missingAccount.CreateHoldFacade()
				Expect(errors.Is(err, hold.ErrInvalidHold)).To(BeTrue())

				invalidType := base
				invalidType.Type = "UNKNOWN"
				_, err = invalidType.CreateHoldFacade()
				Expect(errors.Is(err, hold.ErrInvalidHoldType)).To(BeTrue())
			})
			It("should reject zero or invalid requested amounts", func() {
				zero, err := money.NewMoney(0, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.BRL), Origin: "issuer", Reference: "zero", Type: hold.Judicial, Requested: zero}

				_, err = input.CreateHoldFacade()
				Expect(errors.Is(err, hold.ErrInvalidHold)).To(BeTrue())

				input.Requested = money.Money{}
				_, err = input.CreateHoldFacade()
				Expect(err).To(HaveOccurred())
			})
			It("should reject a currency that differs from the requested amount", func() {
				amount, err := money.NewMoney(500, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				input := dto.CreateHoldInput{AccountID: "account-1", Currency: string(money.USD), Origin: "issuer", Reference: "currency", Type: hold.Judicial, Requested: amount}

				_, err = input.CreateHoldFacade()

				Expect(err).To(MatchError(money.ErrCurrencyMismatch))
			})
		})
	})
})
