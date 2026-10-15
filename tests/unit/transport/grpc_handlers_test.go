//go:build unit

package transport_test

import (
	"context"
	"errors"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	grpcHandler "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/handler"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	"github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcAccountRepositoryFake struct{ saved *account.Account }

func (f *grpcAccountRepositoryFake) Save(_ context.Context, value *account.Account) error {
	f.saved = value
	return nil
}
func (*grpcAccountRepositoryFake) FindAccount(context.Context, criteria.AccountCriteria) (*account.Account, error) {
	return nil, nil
}

type grpcEventPublisherFake struct {
	event event.Event
	err   error
}

func (f *grpcEventPublisherFake) Publish(_ context.Context, value event.Event) error {
	f.event = value
	return f.err
}

type grpcTransactionRepositoryFake struct {
	value *transaction.Transaction
	err   error
}

func (*grpcTransactionRepositoryFake) Save(context.Context, *transaction.Transaction) error {
	return nil
}
func (f *grpcTransactionRepositoryFake) Find(context.Context, transaction.TransactionCriteria) (*transaction.Transaction, error) {
	return f.value, f.err
}
func (*grpcTransactionRepositoryFake) FindLatestLedgerState(context.Context, string) (int64, int64, error) {
	return 0, 0, nil
}
func (*grpcTransactionRepositoryFake) ExistsByIdempotencyKey(context.Context, string) (bool, error) {
	return false, nil
}

var _ account.Repository = (*grpcAccountRepositoryFake)(nil)
var _ event.Publisher = (*grpcEventPublisherFake)(nil)
var _ transaction.Repository = (*grpcTransactionRepositoryFake)(nil)

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: HANDLER :: CREATE ACCOUNT", func() {
	Describe("#Handle", func() {
		Context("success cases", func() {
			It("should map a valid request and return the created account id", func() {
				// Arrange.
				repo := &grpcAccountRepositoryFake{}
				publisher := &grpcEventPublisherFake{}
				useCase := command.NewCreateAccount(repo, publisher, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{}, adapter.SilentMetricsMock{})
				sut := grpcHandler.NewCreateAccountHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})
				request := &pb.CreateAccountRequest{
					AccountExternalId: "d589965c-1622-4329-98f9-f13354a2e4dc",
					AccountNumber:     "123456",
					TaxId:             "529.982.247-25",
					AccountType:       pb.AccountType_ACCOUNT_TYPE_ASSET,
					BalancePolicy:     pb.BalancePolicy_BALANCE_POLICY_NON_NEGATIVE,
					Currency:          pb.Currency_CURRENCY_BRL,
				}

				// Act.
				response, err := sut.Handle(context.Background(), request)

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(response.GetAccountId()).To(Equal(repo.saved.ID.String()))
				Expect(repo.saved.AccountType).To(Equal(account.Asset))
				Expect(repo.saved.BalancePolicy).To(Equal(account.BalanceNonNegative))
				Expect(publisher.event).NotTo(BeNil())
			})
		})

		Context("error cases", func() {
			It("should return an invalid argument status for invalid account fields", func() {
				// Arrange.
				useCase := command.NewCreateAccount(&grpcAccountRepositoryFake{}, &grpcEventPublisherFake{}, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{}, adapter.SilentMetricsMock{})
				sut := grpcHandler.NewCreateAccountHandler(useCase, adapter.SilentLoggerMock{}, adapter.SilentTracerMock{})

				// Act.
				_, err := sut.Handle(context.Background(), &pb.CreateAccountRequest{})

				// Assert.
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			})
		})
	})

	Describe("#BalancePolicyTranslate", func() {
		Context("success cases", func() {
			It("should map supported policies and leave unknown values empty", func() {
				// Arrange.
				sut := grpcHandler.NewCreateAccountHandler(nil, nil, nil)

				// Act / Assert.
				Expect(sut.BalancePolicyTranslate(&pb.CreateAccountRequest{BalancePolicy: pb.BalancePolicy_BALANCE_POLICY_NON_NEGATIVE})).To(Equal(account.BalanceNonNegative))
				Expect(sut.BalancePolicyTranslate(&pb.CreateAccountRequest{BalancePolicy: pb.BalancePolicy_BALANCE_POLICY_UNRESTRICTED})).To(Equal(account.BalanceUnrestricted))
				Expect(sut.BalancePolicyTranslate(&pb.CreateAccountRequest{BalancePolicy: pb.BalancePolicy(99)})).To(BeEmpty())
			})
		})
	})

	Describe("#AccountTypeTranslate", func() {
		Context("success cases", func() {
			It("should map each accounting type and leave unknown values empty", func() {
				// Arrange.
				sut := grpcHandler.NewCreateAccountHandler(nil, nil, nil)

				// Act / Assert.
				values := []struct {
					input pb.AccountType
					want  account.Type
				}{
					{pb.AccountType_ACCOUNT_TYPE_ASSET, account.Asset},
					{pb.AccountType_ACCOUNT_TYPE_LIABILITY, account.Liability},
					{pb.AccountType_ACCOUNT_TYPE_EQUITY, account.Equity},
					{pb.AccountType_ACCOUNT_TYPE_REVENUE, account.Revenue},
					{pb.AccountType_ACCOUNT_TYPE_EXPENSE, account.Expense},
				}
				for _, value := range values {
					Expect(sut.AccountTypeTranslate(&pb.CreateAccountRequest{AccountType: value.input})).To(Equal(value.want))
				}
				Expect(sut.AccountTypeTranslate(&pb.CreateAccountRequest{AccountType: pb.AccountType(99)})).To(BeEmpty())
			})
		})
	})

	Describe("#CurrencyTranslate", func() {
		Context("success cases", func() {
			It("should map supported currencies and leave unknown values empty", func() {
				// Arrange.
				sut := grpcHandler.NewCreateAccountHandler(nil, nil, nil)

				// Act / Assert.
				Expect(sut.CurrencyTranslate(&pb.CreateAccountRequest{Currency: pb.Currency_CURRENCY_BRL})).To(Equal(money.BRL))
				Expect(sut.CurrencyTranslate(&pb.CreateAccountRequest{Currency: pb.Currency_CURRENCY_USD})).To(Equal(money.USD))
				Expect(sut.CurrencyTranslate(&pb.CreateAccountRequest{Currency: pb.Currency_CURRENCY_EUR})).To(Equal(money.EUR))
				Expect(sut.CurrencyTranslate(&pb.CreateAccountRequest{Currency: pb.Currency(99)})).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: HANDLER :: GET TRANSACTION", func() {
	Describe("#Handle", func() {
		Context("success cases", func() {
			It("should map transaction and entry data into the protobuf response", func() {
				// Arrange.
				created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				amount, err := money.NewMoney(100, money.BRL)
				Expect(err).NotTo(HaveOccurred())
				entry, err := transaction.NewEntryBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b02").
					WithTransactionID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4dc").
					WithDirection(transaction.Debit).
					WithAmount(amount).
					WithCreatedAt(created).
					Build()
				Expect(err).NotTo(HaveOccurred())
				entry.AssignAccountID("d589965c-1622-4329-98f9-f13354a2e4dc")
				entry.SetSequenceNumber(4)
				entry.SetTransactionPosition(2)
				credit, err := transaction.NewEntryBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b03").
					WithTransactionID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithAccountExternalID("d589965c-1622-4329-98f9-f13354a2e4de").
					WithDirection(transaction.Credit).
					WithAmount(amount).
					WithCreatedAt(created).
					Build()
				Expect(err).NotTo(HaveOccurred())
				credit.AssignAccountID("d589965c-1622-4329-98f9-f13354a2e4de")
				credit.SetSequenceNumber(5)
				credit.SetTransactionPosition(3)
				transactionEntity, err := transaction.NewTransactionBuilder().
					WithID("019ff448-c43d-70d3-83c7-dfa067446b01").
					WithStatus(transaction.Completed).
					WithOperation(transaction.OperationDeposit).
					WithAmount(amount).
					WithCreatedAt(created).
					WithUpdatedAt(created.Add(time.Minute)).
					WithEntries([]*transaction.Entry{entry, credit}).
					Build()
				Expect(err).NotTo(HaveOccurred())
				sut := grpcHandler.NewGetTransactionHandler(command.NewGetTransaction(&grpcTransactionRepositoryFake{value: transactionEntity}))

				// Act.
				response, err := sut.Handle(context.Background(), &pb.GetTransactionRequest{TransactionId: transactionEntity.ID.String()})

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(response.TransactionId).To(Equal(transactionEntity.ID.String()))
				Expect(response.Status).To(Equal(string(transaction.Completed)))
				Expect(response.Entries).To(HaveLen(2))
				Expect(response.Entries[0].EntryId).To(Equal(entry.ID.String()))
				Expect(response.Entries[0].SequenceNumber).To(Equal(int64(4)))
				Expect(response.Entries[0].Position).To(Equal(int64(2)))
			})
		})

		Context("error cases", func() {
			It("should translate repository failures to a gRPC status", func() {
				// Arrange.
				wantErr := errors.New("query failed")
				sut := grpcHandler.NewGetTransactionHandler(command.NewGetTransaction(&grpcTransactionRepositoryFake{err: wantErr}))

				// Act.
				_, err := sut.Handle(context.Background(), &pb.GetTransactionRequest{TransactionId: "tx"})

				// Assert.
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).NotTo(Equal(codes.OK))
			})
		})
	})
})
