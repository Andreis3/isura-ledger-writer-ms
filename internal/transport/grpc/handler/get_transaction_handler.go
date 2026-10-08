package handler

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/translator"
)

type GetTransactionHandler struct {
	useCase *command.GetTransaction
}

func NewGetTransactionHandler(useCase *command.GetTransaction) *GetTransactionHandler {
	return &GetTransactionHandler{useCase: useCase}
}

func (h *GetTransactionHandler) Handle(ctx context.Context, req *pb.GetTransactionRequest) (*pb.GetTransactionResponse, error) {
	result, err := h.useCase.Execute(ctx, req.GetTransactionId())
	if err != nil {
		return nil, translator.ToGRPCError(err)
	}
	entries := make([]*pb.EntryResponse, 0, len(result.Entries))
	for _, entry := range result.Entries {
		entries = append(entries, &pb.EntryResponse{
			EntryId:        entry.ID.String(),
			Direction:      entry.Direction.String(),
			Amount:         entry.Amount.Amount(),
			Currency:       string(entry.Amount.Currency()),
			AccountId:      entry.AccountExternalID,
			Position:       entry.TransactionPosition,
			SequenceNumber: entry.SequenceNumber,
		})
	}
	return &pb.GetTransactionResponse{
		TransactionId: result.ID.String(),
		Status:        string(result.Status),
		CreatedAt:     result.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:     result.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Entries:       entries,
	}, nil
}
