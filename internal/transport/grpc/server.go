package grpc

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/handler"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
)

type Handlers map[string]any

type LedgerServer struct {
	pb.UnimplementedLedgerServiceServer
	createAccount     *handler.CreateAccountHandler
	createTransaction *handler.CreateTransactionHandler
}

func NewLedgerServer(createAccount *handler.CreateAccountHandler, createTransaction *handler.CreateTransactionHandler) *LedgerServer {
	return &LedgerServer{
		createAccount:     createAccount,
		createTransaction: createTransaction,
	}
}

func (s *LedgerServer) CreateTransaction(ctx context.Context, req *pb.CreateTransactionRequest) (*pb.CreateTransactionResponse, error) {
	return s.createTransaction.Handle(ctx, req)
}

func (s *LedgerServer) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	return s.createAccount.Handle(ctx, req)
}
