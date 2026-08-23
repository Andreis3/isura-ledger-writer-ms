package main

import (
	_ "net/http/pprof" // <-- Automatically imports pprof handlers

	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/server"
)

func main() {
	deps := dependency.BuildBaseDeps()

	server.StartServersWithGracefulShutdown(deps)
}
