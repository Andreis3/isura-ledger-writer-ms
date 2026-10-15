package handler

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/translator"
)

type CreateTransactionHandler struct {
	useCase createTransactionUseCase
	log     application.Logger
	tracer  application.Tracer
}

type createTransactionUseCase interface {
	Execute(context.Context, dto.CreateTransactionInput) (*dto.CreateTransactionOutput, error)
}

func NewCreateTransactionHandler(
	useCase createTransactionUseCase,
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

	var entries []dto.EntryInput
	if len(req.GetEntries()) > 0 {
		entries = make([]dto.EntryInput, 0, len(req.GetEntries()))
	}
	for _, entry := range req.GetEntries() {
		entries = append(entries, dto.EntryInput{
			AccountID: entry.GetAccountId(),
			Direction: entry.GetDirection(),
			Amount:    entry.GetAmount(),
			Currency:  entry.GetCurrency(),
			Metadata:  entry.GetMetadata(),
		})
	}
	var movements []dto.MovementInput
	if len(req.GetMovements()) > 0 {
		movements = make([]dto.MovementInput, 0, len(req.GetMovements()))
	}
	for _, movement := range req.GetMovements() {
		positions := make([]int, 0, len(movement.GetEntryPositions()))
		for _, position := range movement.GetEntryPositions() {
			positions = append(positions, int(position))
		}
		movements = append(movements, dto.MovementInput{
			Type: movement.GetType(), Description: movement.GetDescription(), EntryPositions: positions,
		})
	}
	input := dto.CreateTransactionInput{
		IdempotencyKey:  stringPointer(req.GetIdempotencyKey()),
		Entries:         entries,
		Movements:       movements,
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
