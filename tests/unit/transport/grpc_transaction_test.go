//go:build unit

package transport_test

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	grpcHandler "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/handler"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	adapter "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type createTransactionGRPCFake struct {
	input dto.CreateTransactionInput
}

func (f *createTransactionGRPCFake) Execute(_ context.Context, input dto.CreateTransactionInput) (*dto.CreateTransactionOutput, error) {
	f.input = input
	return &dto.CreateTransactionOutput{Status: "COMPLETED"}, nil
}

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: CREATE TRANSACTION HANDLER", func() {
	Describe("#Handle", func() {
		Context("success cases", func() {
			It("should map movement fields and zero-based entry positions to the application input", func() {
				// Arrange
				useCase := &createTransactionGRPCFake{}
				sut := grpcHandler.NewCreateTransactionHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := &pb.CreateTransactionRequest{
					Movements: []*pb.MovementRequest{{
						Type: "TRANSFER", Description: "main transfer", EntryPositions: []int32{0, 1},
					}},
				}

				// Act
				_, err := sut.Handle(context.Background(), request)

				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(useCase.input.Movements).To(Equal([]dto.MovementInput{{
					Type: "TRANSFER", Description: "main transfer", EntryPositions: []int{0, 1},
				}}))
			})
		})
	})
})
