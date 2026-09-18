package handler

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/translator"
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

func (h *CreateTransactionHandler) Handle(ctx context.Context, req *pb.CreateTransactionRequest) (*pb.CreateTransactionResponse, error) {
	ctx, span := h.tracer.Start(ctx, "CreateTransactionHandler.Handle")
	defer span.End()

	input := dto.CreateTransactionInput{
		IdempotencyKey:  stringPointer(req.GetIdempotencyKey()),
		DebitAccountID:  stringPointer(req.GetDebitAccountId()),
		CreditAccountID: stringPointer(req.GetCreditAccountId()),
		Amount:          int64Pointer(req.GetAmount()),
		Currency:        stringPointer(req.GetCurrency()),
		Operation:       stringPointer(req.GetOperation()),
		Metadata:        req.GetMetadata(),
	}
	response, err := h.useCase.Execute(ctx, input)
	if err != nil {
		return nil, translator.ToGRPCError(err)
	}
	return &pb.CreateTransactionResponse{
		TransactionId:    valueOrEmpty(response.TransactionID),
		Status:           response.Status,
		IdempotentReplay: response.IdempotentReplay,
	}, nil
}

func stringPointer(value string) *string { return &value }

func int64Pointer(value int64) *int64 { return &value }

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
