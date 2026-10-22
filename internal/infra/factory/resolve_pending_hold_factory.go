package factory

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
)

// NewResolvePendingHoldFactory composes the internally authorized resolution command.
func NewResolvePendingHoldFactory(baseDeps *dependency.BaseDeps, policies command.HoldPolicyProvider) *command.ResolvePendingHold {
	composer := dependency.NewComposer(baseDeps)
	return command.NewResolvePendingHold(uow.NewUnitOfWork(baseDeps.Pg.Pool(), baseDeps.Prom), composer.BuildAccountRepo(),
		composer.BuildTransactionRepo(), composer.BuildHoldRepo(), policies, composer.BuildOutboxRepo(), baseDeps.Tracer, baseDeps.Log, baseDeps.Prom)
}
