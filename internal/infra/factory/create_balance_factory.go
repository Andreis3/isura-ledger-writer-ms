package factory

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue/handler"
)

func NewCreateBalanceFactory(
	baseDeps *dependency.BaseDeps,
) *handler.CreateBalanceHandler {
	composeBuild := dependency.NewComposer(baseDeps)
	balanceCommand := command.NewCreateBalance(
		composeBuild.BuildBalance(),
		composeBuild.BuildAccountRepo(),
		baseDeps.Log,
		baseDeps.Tracer,
		baseDeps.Prom,
	)

	handler := handler.NewCreateBalanceHandler(balanceCommand, baseDeps.Log, baseDeps.Tracer)

	return handler
}
