package factory

import (
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
)

// NewReleaseHoldFactory composes the internally authorized hold release command.
func NewReleaseHoldFactory(baseDeps *dependency.BaseDeps) *command.ReleaseHold {
	composer := dependency.NewComposer(baseDeps)
	return command.NewReleaseHold(
		uow.NewUnitOfWork(baseDeps.Pg.Pool(), baseDeps.Prom),
		composer.BuildHoldRepo(),
		composer.BuildOutboxRepo(),
		baseDeps.Tracer,
		baseDeps.Log,
		baseDeps.Prom,
	)
}
