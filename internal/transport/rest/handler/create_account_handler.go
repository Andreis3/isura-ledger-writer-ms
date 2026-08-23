package handler

import (
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/decoder"
)

type CreateAccountHandler struct {
	useCase *command.CreateAccount
	log     application.Logger
	tracer  application.Tracer
}

func NewCreateAccountHandler(
	useCase *command.CreateAccount,
	log application.Logger,
	tracer application.Tracer,
) *CreateAccountHandler {
	return &CreateAccountHandler{
		useCase: useCase,
		log:     log,
		tracer:  tracer,
	}
}

func (h *CreateAccountHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "CreateAccountHandler.Handle")

	input, err := decoder.RequestDecoder[dto.CreateAccountInput](r)
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

	decoder.ResponseSuccess[dto.CreateAccountOutput](w, http.StatusCreated, *response)

}
