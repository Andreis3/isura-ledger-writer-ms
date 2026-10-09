package dependency

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	"github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/internal/infra/observability"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres"
)

type BaseDeps struct {
	Cfg            *configs.Configs
	Log            *logger.Logger
	Prom           *observability.Prometheus
	Pg             *postgres.Postgres
	Tracer         application.Tracer
	Nats           *nats.ClientNats
	TracerShutdown func(context.Context) error
}

const postgresStartupTimeout = 10 * time.Second

func BuildBaseDeps() *BaseDeps {
	cfg, err := configs.LoadConfig()
	log := logger.NewLogger()
	if err != nil {
		log.CriticalText("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	prom, err := observability.NewPrometheus()
	if err != nil {
		log.CriticalText("failed to initialize Prometheus", slog.String("error", err.Error()))
		os.Exit(1)
	}

	postgresCtx, cancelPostgres := context.WithTimeout(context.Background(), postgresStartupTimeout)
	defer cancelPostgres()
	pg, err := postgres.NewPostgresWithContext(postgresCtx, cfg)
	if err != nil {
		log.CriticalText("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	tracer, tracerShutdown, err := observability.InitOtelTracer(context.Background(), cfg)
	if err != nil {
		log.CriticalText("failed to initialize tracer", slog.String("error", err.Error()))
		os.Exit(1)
	}

	nats, err := nats.NewJetStreamConnection(cfg)
	if err != nil {
		log.CriticalText("failed to connect to nats", slog.String("error", err.Error()))
		os.Exit(1)
	}

	return &BaseDeps{
		Cfg:            cfg,
		Log:            log,
		Prom:           prom,
		Pg:             pg,
		Tracer:         tracer,
		Nats:           nats,
		TracerShutdown: tracerShutdown,
	}
}
