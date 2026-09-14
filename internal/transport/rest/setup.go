package rest

import (
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/factory"
	"github.com/go-chi/chi/v5"

	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/module"
)

type SetupDeps struct {
	Mux  *chi.Mux
	Deps *dependency.BaseDeps
}

func Setup(st *SetupDeps) {
	NewRegisterRoutes(
		st.Mux,
		st.Deps.Log,
		BuildRoutes(st),
	).Register()
}

func BuildRoutes(st *SetupDeps) []ModuleRoutes {
	return []ModuleRoutes{
		module.NewMetrics(),
		module.NewPPROF(),
		module.NewHealthCheck(st.Deps.Pg, st.Deps.Cfg.ApplicationName),
		module.NewAccountModule(*st.Deps, factory.NewCreateAccountFactory(st.Deps)),
		module.NewTransactionModule(*st.Deps, factory.NewCreateTransactionFactory(st.Deps)),
	}
}
