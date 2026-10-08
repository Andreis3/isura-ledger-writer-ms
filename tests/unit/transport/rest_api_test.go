//go:build unit

package transport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/handler"
	adapter "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type createAccountAPIFake struct {
	output *dto.CreateAccountOutput
	err    error
	input  dto.CreateAccountInput
	calls  int
}

func (f *createAccountAPIFake) Execute(_ context.Context, input dto.CreateAccountInput) (*dto.CreateAccountOutput, error) {
	f.calls++
	f.input = input
	return f.output, f.err
}

type createTransactionAPIFake struct {
	output *dto.CreateTransactionOutput
	err    error
	input  dto.CreateTransactionInput
	calls  int
}

func (f *createTransactionAPIFake) Execute(_ context.Context, input dto.CreateTransactionInput) (*dto.CreateTransactionOutput, error) {
	f.calls++
	f.input = input
	return f.output, f.err
}

var _ = Describe("INTERNAL :: TRANSPORT :: REST :: API", func() {
	Describe("#CreateAccount", func() {
		Context("success cases", func() {
			It("should return the created account response with HTTP 201", func() {
				// Arrange
				accountID := "account-123"
				useCase := &createAccountAPIFake{output: &dto.CreateAccountOutput{AccountID: &accountID}}
				sut := handler.NewCreateAccountHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"account_external_id":"external-1"}`))
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusCreated))
				Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
				Expect(response.Body.String()).To(ContainSubstring(`"account_id":"account-123"`))
				Expect(useCase.calls).To(Equal(1))
				Expect(useCase.input.AccountExternalID).To(Equal("external-1"))
			})
		})

		Context("error cases", func() {
			It("should reject unknown request fields without executing the use case", func() {
				// Arrange
				useCase := &createAccountAPIFake{}
				sut := handler.NewCreateAccountHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"unexpected":"value"}`))
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusBadRequest))
				Expect(useCase.calls).To(BeZero())
			})
		})
	})

	Describe("#CreateTransaction", func() {
		Context("success cases", func() {
			It("should map multiple ordered entries and preserve their metadata", func() {
				// Arrange
				useCase := &createTransactionAPIFake{output: &dto.CreateTransactionOutput{Status: "COMPLETED"}}
				sut := handler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{"amount":100,"operation":"TRANSFER","entries":[{"account_id":"account-a","direction":"DEBIT","amount":100,"currency":"BRL","metadata":{"source":"api"}},{"account_id":"account-b","direction":"CREDIT","amount":100,"currency":"BRL"}]}`))
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusCreated))
				Expect(useCase.input.Entries).To(HaveLen(2))
				Expect(useCase.input.Entries[0].Direction).To(Equal("DEBIT"))
				Expect(useCase.input.Entries[0].Metadata).To(HaveKeyWithValue("source", "api"))
				Expect(useCase.input.Entries[1].AccountID).To(Equal("account-b"))
			})

			It("should accept the idempotency header and return HTTP 201", func() {
				// Arrange
				transactionID := "transaction-123"
				useCase := &createTransactionAPIFake{output: &dto.CreateTransactionOutput{TransactionID: &transactionID, Status: "completed"}}
				sut := handler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{"debit_account_id":"debit","credit_account_id":"credit","amount":10,"currency":"USD","operation":"transfer"}`))
				request.Header.Set("Idempotency-Key", "request-123")
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusCreated))
				Expect(response.Body.String()).To(ContainSubstring(`"transaction_id":"transaction-123"`))
				Expect(useCase.calls).To(Equal(1))
				Expect(useCase.input.IdempotencyKey).NotTo(BeNil())
				Expect(*useCase.input.IdempotencyKey).To(Equal("request-123"))
			})

			It("should return HTTP 200 when the use case reports an idempotent replay", func() {
				// Arrange
				transactionID := "transaction-123"
				useCase := &createTransactionAPIFake{output: &dto.CreateTransactionOutput{TransactionID: &transactionID, Status: "completed", IdempotentReplay: true}}
				sut := handler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{}`))
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusOK))
				Expect(response.Body.String()).To(ContainSubstring(`"idempotent_replay":true`))
			})
		})

		Context("error cases", func() {
			It("should reject a mismatched idempotency header and body", func() {
				// Arrange
				useCase := &createTransactionAPIFake{}
				sut := handler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{"idempotency_key":"body-key"}`))
				request.Header.Set("Idempotency-Key", "header-key")
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusBadRequest))
				Expect(response.Body.String()).To(ContainSubstring("idempotency_key"))
				Expect(useCase.calls).To(BeZero())
			})

			It("should translate use case failures into safe API errors", func() {
				// Arrange
				useCase := &createTransactionAPIFake{err: fault.IdempotencyConflictError(errors.New("private database detail"))}
				sut := handler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{}`))
				response := httptest.NewRecorder()

				// Act
				sut.Handle(response, request)

				// Assert
				Expect(response.Code).To(Equal(http.StatusConflict))
				Expect(response.Body.String()).NotTo(ContainSubstring("private database detail"))
			})
		})
	})
})
