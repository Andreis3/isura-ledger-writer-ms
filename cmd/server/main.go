package main

import (
	_ "net/http/pprof" // <-- Automatically imports pprof handlers

	"github.com/andreis3/isura-ledger-ms/internal/infra/server"
)

func main() {
	server.StartServersWithGracefulShutdown()
}
