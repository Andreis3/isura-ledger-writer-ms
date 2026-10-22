package factory

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/query"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
)

// NewCreateHoldFactory composes the internal hold command with an explicit policy provider.
func NewCreateHoldFactory(baseDeps *dependency.BaseDeps, policies command.HoldPolicyProvider) *command.CreateHold {
	composer := dependency.NewComposer(baseDeps)
	return command.NewCreateHold(
		uow.NewUnitOfWork(baseDeps.Pg.Pool(), baseDeps.Prom),
		composer.BuildAccountRepo(),
		composer.BuildTransactionRepo(),
		composer.BuildHoldRepo(),
		policies,
		composer.BuildOutboxRepo(),
	)
}

// NewGetHoldFactory composes the internal hold state and history query.
func NewGetHoldFactory(baseDeps *dependency.BaseDeps) *query.GetHold {
	return query.NewGetHold(dependency.NewComposer(baseDeps).BuildHoldRepo())
}
