package dependency

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/balance"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/observability"
)

type Composer struct {
	deps *BaseDeps
}

func NewComposer(baseDeps *BaseDeps) *Composer {
	return &Composer{
		deps: baseDeps,
	}
}

func (c *Composer) BuildAccountRepo() account.Repository {
	return observability.NewObservabilityAccountRepo(
		repository.NewAccountRepository(c.deps.Pg),
		c.deps.Prom,
		c.deps.Tracer,
	)
}

func (c *Composer) BuildTransactionRepo() transaction.Repository {
	return observability.NewObservabilityTransactionRepo(
		repository.NewTransactionRepository(c.deps.Pg),
		c.deps.Prom,
		c.deps.Tracer,
	)
}

func (c *Composer) BuildOutboxRepo() outbox.Repository {
	return observability.NewObservabilityOutboxRepo(
		repository.NewOutBoxRepository(c.deps.Pg),
		c.deps.Prom,
		c.deps.Tracer,
	)
}

func (c *Composer) BuildBalance() balance.Repository {
	return observability.NewObservabilityBalanceRepo(
		repository.NewBalanceRepository(c.deps.Pg),
		c.deps.Prom,
		c.deps.Tracer,
	)
}

func (c *Composer) BuildNatsPublisher() event.Publisher {
	return nats.NewJetStreamPublisher(c.deps.Nats.JS, c.deps.Tracer)
}
