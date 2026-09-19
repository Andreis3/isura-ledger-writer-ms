package handler

import (
	"errors"
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/decoder"
)

type CreateTransactionHandler struct {
	useCase *command.CreateTransaction
	log     application.Logger
	tracer  application.Tracer
}

func NewCreateTransactionHandler(
	useCase *command.CreateTransaction,
	log application.Logger,
	tracer application.Tracer,
) *CreateTransactionHandler {
	return &CreateTransactionHandler{
		useCase: useCase,
		log:     log,
		tracer:  tracer,
	}
}

func (h *CreateTransactionHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "CreateTransactionHandler.Handle")

	input, err := decoder.RequestDecoder[dto.CreateTransactionInput](r)
	if err != nil {
		span.RecordError(err)
		decoder.ResponseError(w, err)
		return
	}
	if err := applyIdempotencyHeader(r, &input); err != nil {
		span.RecordError(err)
		decoder.ResponseError(w, err)
		return
	}

	response, err := h.useCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		decoder.ResponseError(w, err)
		return
	}

	statusCode := http.StatusCreated
	if response.IdempotentReplay {
		statusCode = http.StatusOK
	}
	decoder.ResponseSuccess[dto.CreateTransactionOutput](w, statusCode, *response)

}

func applyIdempotencyHeader(r *http.Request, input *dto.CreateTransactionInput) error {
	headerKey := r.Header.Get("Idempotency-Key")
	if headerKey == "" {
		return nil
	}
	if input.IdempotencyKey != nil && *input.IdempotencyKey != headerKey {
		return fault.InvalidEntityError(errors.New("idempotency key differs between header and body"), map[string]any{
			"idempotency_key": "header and body values must match",
		})
	}
	if input.IdempotencyKey == nil {
		input.IdempotencyKey = &headerKey
	}
	return nil
}
