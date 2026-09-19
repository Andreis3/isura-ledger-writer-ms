package server

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/handler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcTransport "github.com/andreis3/isura-ledger-ms/internal/transport/grpc"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/interceptor"
)

type GRPCServer struct {
	grpcServer *grpc.Server
	deps       *dependency.BaseDeps
}

func NewGRPCServer(
	deps *dependency.BaseDeps,
) *GRPCServer {

	return &GRPCServer{
		deps: deps,
	}
}

func (s *GRPCServer) Start() error {
	start := time.Now()

	// GRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.LoggingInterceptor(s.deps.Log.SlogJSON()),
			interceptor.MetricsInterceptor(s.deps.Prom),
			interceptor.TracingInterceptor(s.deps.Tracer),
		),
	)
	s.grpcServer = grpcServer

	// registers all modules
	registry := grpcTransport.NewServerRegistry(grpcServer, grpcTransport.NewLedgerModule(s.buildLedgerServer()))
	registry.RegisterAll()
	reflection.Register(grpcServer)

	s.deps.Log.InfoText("GRPC server started",
		slog.String("port", s.deps.Cfg.Servers.GRPC.Port),
		slog.String("startup_time", time.Since(start).String()),
	)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", s.deps.Cfg.Servers.GRPC.Port))
	if err != nil {
		s.deps.Log.CriticalText("grpc server failed to listen",
			slog.String("error", err.Error()))
		return fmt.Errorf("listen grpc server: %w", err)
	}

	if err := grpcServer.Serve(lis); err != nil {
		s.deps.Log.CriticalText("grpc server failed to serve",
			slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (s *GRPCServer) buildLedgerServer() *grpcTransport.LedgerServer {

	composer := dependency.NewComposer(s.deps)

	accountRepo := composer.BuildAccountRepo()

	publisher := composer.BuildNatsPublisher()

	// use cases
	createAccount := command.NewCreateAccount(accountRepo, publisher, s.deps.Log, s.deps.Tracer, s.deps.Prom)
	createTransaction := command.NewCreateTransaction(
		uow.NewUnitOfWork(s.deps.Pg.Pool()),
		accountRepo,
		composer.BuildTransactionRepo(),
		composer.BuildOutboxRepo(),
		s.deps.Tracer,
		s.deps.Log,
		s.deps.Prom,
	)

	// handlers
	createAccountHandler := handler.NewCreateAccountHandler(createAccount, s.deps.Log, s.deps.Tracer)
	createTransactionHandler := handler.NewCreateTransactionHandler(createTransaction, s.deps.Log, s.deps.Tracer)

	// server
	ledgerServer := grpcTransport.NewLedgerServer(createAccountHandler, createTransactionHandler)

	// server
	return ledgerServer
}

func (s *GRPCServer) GracefulStop() {
	if s.grpcServer != nil {
		s.deps.Log.InfoText("GRPC server stopped")
		s.grpcServer.GracefulStop()
	}
}
