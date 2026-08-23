package handler

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue"
	"github.com/andreis3/isura-ledger-ms/internal/util"
	"github.com/nats-io/nats.go/jetstream"
)

type CreateBalanceHandler struct {
	useCase *command.CreateBalance
	log     application.Logger
	tracer  application.Tracer
}

func NewCreateBalanceHandler(
	useCase *command.CreateBalance,
	log application.Logger,
	tracer application.Tracer,
) *CreateBalanceHandler {
	return &CreateBalanceHandler{
		useCase: useCase,
		log:     log,
		tracer:  tracer,
	}
}

func (h *CreateBalanceHandler) Handle(ctx context.Context, msg jetstream.Msg) error {
	ctx, span := h.tracer.Start(ctx, "CreateBalanceHandler.Handle")
	defer span.End()

	var input dto.CreateBalanceInput
	if err := util.JsonEngine.Unmarshal(msg.Data(), &input); err != nil {
		h.log.ErrorJSON("Failed to unmarshal account created event", "error", err.Error())
		return queue.NewPermanentError(err)
	}

	err := h.useCase.Execute(ctx, input)
	if err != nil {
		h.log.ErrorJSON("Failed to process create balance from event", "error", err.Error(), "account_id", input.AccountID)
		return queue.NewTransientError(err, 10*time.Second, false)
	}

	return nil // O NatsConsumerServer trata o Ack() em caso de sucesso
}
