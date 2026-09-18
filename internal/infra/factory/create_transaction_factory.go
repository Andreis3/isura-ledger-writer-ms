package factory

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/handler"
)

func NewCreateTransactionFactory(
	baseDeps *dependency.BaseDeps,
) *handler.CreateTransactionHandler {
	composeBuild := dependency.NewComposer(baseDeps)
	uowDep := uow.NewUnitOfWork(baseDeps.Pg.Pool(), baseDeps.Prom)
	transactionCommand := command.NewCreateTransaction(
		uowDep,
		composeBuild.BuildAccountRepo(),
		composeBuild.BuildTransactionRepo(),
		composeBuild.BuildOutboxRepo(),
		baseDeps.Tracer,
		baseDeps.Log,
		baseDeps.Prom,
	)

	createTransactionHandler := handler.NewCreateTransactionHandler(transactionCommand, baseDeps.Log, baseDeps.Tracer)

	return createTransactionHandler
}
