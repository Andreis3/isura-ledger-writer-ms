package server

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"golang.org/x/sync/errgroup"
)

func StartServersWithGracefulShutdown(deps *dependency.BaseDeps) {
	// Inicializa o composer para centralizar a criação de dependências de infra/domínio
	composer := dependency.NewComposer(deps)

	// Constrói o publisher usando o método padronizado do projeto
	natsPublisher := composer.BuildNatsPublisher()

	grpcSrv := NewGRPCServer(deps)
	httpSrv := NewHTTPServer(deps)

	natsConsumer := NewNatsConsumerServer(deps, natsPublisher)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	// Goroutine for the HTTP server
	g.Go(func() error {
		// Starts the server in a separate goroutine
		errCh := make(chan error, 1)
		go func() {
			deps.Log.InfoText("Starting HTTP server...")
			if err := httpSrv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
			close(errCh)
		}()

		// Waits for the context to be canceled or an error on the server
		select {
		case <-ctx.Done():
			deps.Log.InfoText("Shutting down HTTP server...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return httpSrv.Shutdown(shutdownCtx) // This stops the server
		case err := <-errCh:
			return err
		}
	})

	// Goroutine for the gRPC server
	g.Go(func() error {
		errCh := make(chan error, 1)
		go func() {
			deps.Log.InfoText("Starting GRPC server...")
			if err := grpcSrv.Start(); err != nil {
				errCh <- err
			}
			close(errCh)
		}()

		select {
		case <-ctx.Done():
			deps.Log.InfoText("Shutting down GRPC server...")
			grpcSrv.GracefulStop()
			return nil
		case err := <-errCh:
			return err
		}
	})

	// Goroutine for the NATS Consumer worker
	g.Go(func() error {
		errCh := make(chan error, 1)
		go func() {
			deps.Log.InfoText("Starting NATS consumer...")
			if err := natsConsumer.Start(ctx); err != nil {
				errCh <- err
			}
		}()

		select {
		case <-ctx.Done():
			deps.Log.InfoText("Shutting down NATS consumer...")
			return nil
		}
	})

	// Waits for all to finish
	if err := g.Wait(); err != nil {
		deps.Log.InfoText("Servers stopped with error: %v", err)
	}

	// Closes infrastructure
	deps.Log.InfoText("Closing infrastructure...")
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	deps.Pg.Close()
	deps.Prom.Close()
	deps.TracerShutdown(closeCtx)
	deps.Nats.Close()
	deps.Log.InfoText("Infrastructure closed.")
	deps.Log.InfoText("Shutdown complete.")
}
