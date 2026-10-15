//go:build unit

package transport_test

import (
	"context"
	"net"
	"time"

	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type ledgerServiceContractFake struct {
	pb.UnimplementedLedgerServiceServer
}

func (ledgerServiceContractFake) CreateTransaction(context.Context, *pb.CreateTransactionRequest) (*pb.CreateTransactionResponse, error) {
	return &pb.CreateTransactionResponse{TransactionId: "transaction-id", Status: "COMPLETED"}, nil
}
func (ledgerServiceContractFake) GetTransaction(context.Context, *pb.GetTransactionRequest) (*pb.GetTransactionResponse, error) {
	return &pb.GetTransactionResponse{TransactionId: "transaction-id", Status: "COMPLETED"}, nil
}
func (ledgerServiceContractFake) CreateAccount(context.Context, *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	return &pb.CreateAccountResponse{AccountId: "account-id"}, nil
}
func (ledgerServiceContractFake) GetBalance(context.Context, *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	return &pb.GetBalanceResponse{Balance: 250, Currency: "BRL"}, nil
}

var _ pb.LedgerServiceServer = ledgerServiceContractFake{}

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: PB :: LEDGER SERVICE", func() {
	Describe("#RegisterLedgerServiceServer", func() {
		Context("success cases", func() {
			It("should route each generated client call to its service method", func() {
				// Arrange.
				listener := bufconn.Listen(1024 * 1024)
				server := grpc.NewServer()
				pb.RegisterLedgerServiceServer(server, ledgerServiceContractFake{})
				go func() { _ = server.Serve(listener) }()
				DeferCleanup(server.Stop)
				DeferCleanup(func() { _ = listener.Close() })
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				DeferCleanup(cancel)
				conn, err := grpc.DialContext(ctx, "bufnet",
					grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
					grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { Expect(conn.Close()).To(Succeed()) })
				client := pb.NewLedgerServiceClient(conn)

				// Act.
				createdTransaction, transactionErr := client.CreateTransaction(ctx, &pb.CreateTransactionRequest{})
				foundTransaction, findErr := client.GetTransaction(ctx, &pb.GetTransactionRequest{})
				createdAccount, accountErr := client.CreateAccount(ctx, &pb.CreateAccountRequest{})
				balance, balanceErr := client.GetBalance(ctx, &pb.GetBalanceRequest{})

				// Assert.
				Expect(transactionErr).NotTo(HaveOccurred())
				Expect(createdTransaction.GetTransactionId()).To(Equal("transaction-id"))
				Expect(findErr).NotTo(HaveOccurred())
				Expect(foundTransaction.GetStatus()).To(Equal("COMPLETED"))
				Expect(accountErr).NotTo(HaveOccurred())
				Expect(createdAccount.GetAccountId()).To(Equal("account-id"))
				Expect(balanceErr).NotTo(HaveOccurred())
				Expect(balance.GetBalance()).To(Equal(int64(250)))
				Expect(balance.GetCurrency()).To(Equal("BRL"))
			})
		})
	})
})
