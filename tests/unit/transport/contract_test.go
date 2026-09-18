//go:build unit

package transport_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	grpctranslator "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/translator"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/decoder"
	resttranslator "github.com/andreis3/isura-ledger-ms/internal/transport/rest/translator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("transport contracts", func() {
	It("maps stable financial fault codes to REST and gRPC statuses", func() {
		Expect(resttranslator.HTTPStatus(fault.CodeDuplicateTransaction)).To(Equal(http.StatusConflict))
		Expect(resttranslator.HTTPStatus(fault.CodeInsufficientBalance)).To(Equal(http.StatusUnprocessableEntity))
		Expect(grpctranslator.GRPCStatus(fault.IdempotencyConflictError(errors.New("mismatch")))).To(Equal(codes.AlreadyExists))
		Expect(grpctranslator.GRPCStatus(fault.TransactionConflictError(errors.New("serialization")))).To(Equal(codes.Unavailable))
	})

	It("does not expose technical causes in REST errors", func() {
		response := httptest.NewRecorder()
		decoder.ResponseError(response, fault.IdempotencyConflictError(errors.New("secret sql detail")))

		Expect(response.Code).To(Equal(http.StatusConflict))
		Expect(response.Body.String()).To(ContainSubstring("ILMS-1005"))
		Expect(response.Body.String()).ToNot(ContainSubstring("secret sql detail"))
	})

	It("returns the stable gRPC error status and friendly message", func() {
		err := grpctranslator.ToGRPCError(fault.IdempotencyConflictError(errors.New("technical detail")))
		Expect(status.Code(err)).To(Equal(codes.AlreadyExists))
		Expect(status.Convert(err).Message()).To(ContainSubstring("Idempotency key"))
		Expect(status.Convert(err).Message()).ToNot(ContainSubstring("technical detail"))
	})

	It("keeps the error response content type stable", func() {
		response := httptest.NewRecorder()
		decoder.ResponseError(response, fault.InvalidEntityError(errors.New("invalid"), nil))

		Expect(response.Header().Get(decoder.ContentType)).To(Equal(decoder.ApplicationJSON))
		Expect(strings.TrimSpace(response.Body.String())).To(HavePrefix(`{"error":`))
	})
})
