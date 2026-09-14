package module

import (
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/handler"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/middleware"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/types"
	"github.com/andreis3/isura-ledger-ms/internal/util"
)

type AccountModule struct {
	baseDeps *dependency.BaseDeps
	handler  *handler.CreateAccountHandler
}

func NewAccountModule(
	baseDeps dependency.BaseDeps,
	handler *handler.CreateAccountHandler,
) *AccountModule {
	return &AccountModule{
		baseDeps: &baseDeps,
		handler:  handler,
	}
}

func (m *AccountModule) Routes() types.RouteType {

	return types.RouteType{
		{
			Method:  http.MethodPost,
			Path:    "/accounts",
			Handler: m.handler.Handle,
			Type:    util.HandlerFunc,
			Middlewares: &types.Middlewares{
				middleware.Tracing(m.baseDeps.Tracer),
				middleware.Logging(m.baseDeps.Log.SlogJSON()),
				middleware.MetricsMiddleware(m.baseDeps.Prom),
			},
		},
	}
}
