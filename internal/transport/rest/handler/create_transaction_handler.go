package handler

import (
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
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

	response, err := h.useCase.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		decoder.ResponseError(w, err)
		return
	}

	decoder.ResponseSuccess[dto.CreateTransactionOutput](w, http.StatusCreated, *response)

}
