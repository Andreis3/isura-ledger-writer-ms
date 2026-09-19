package handler

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	pb "github.com/andreis3/isura-ledger-ms/internal/transport/grpc/pb/ledger/v1"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/translator"
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

func (h *CreateAccountHandler) Handle(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	ctx, span := h.tracer.Start(ctx, "CreateAccountHandler.Handle")
	defer span.End()

	input := dto.CreateAccountInput{
		AccountExternalID: req.GetAccountExternalId(),
		AccountNumber:     req.GetAccountNumber(),
		TaxID:             req.GetTaxId(),
		AccountType:       string(h.AccountTypeTranslate(req)),
		Currency:          string(h.CurrencyTranslate(req)),
	}

	response, err := h.useCase.Execute(ctx, input)
	if err != nil {
		return nil, translator.ToGRPCError(err)
	}

	return &pb.CreateAccountResponse{
		AccountId: *response.AccountID,
	}, nil
}

func (h *CreateAccountHandler) AccountTypeTranslate(req *pb.CreateAccountRequest) account.Type {
	switch req.GetAccountType() {
	case pb.AccountType_ACCOUNT_TYPE_ASSET:
		return account.Asset
	case pb.AccountType_ACCOUNT_TYPE_LIABILITY:
		return account.Liability
	case pb.AccountType_ACCOUNT_TYPE_EQUITY:
		return account.Equity
	case pb.AccountType_ACCOUNT_TYPE_REVENUE:
		return account.Revenue
	case pb.AccountType_ACCOUNT_TYPE_EXPENSE:
		return account.Expense
	default:
		return ""

	}
}

func (h *CreateAccountHandler) CurrencyTranslate(req *pb.CreateAccountRequest) money.Currency {
	switch req.GetCurrency() {
	case pb.Currency_CURRENCY_BRL:
		return money.BRL
	case pb.Currency_CURRENCY_USD:
		return money.USD
	case pb.Currency_CURRENCY_EUR:
		return money.EUR
	default:
		return ""

	}
}
