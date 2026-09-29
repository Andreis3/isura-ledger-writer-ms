package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(conf *configs.Configs) (*Postgres, error) {
	return NewPostgresWithContext(context.Background(), conf)
}

// NewPostgresWithContext creates a PostgreSQL pool and validates connectivity within ctx.
func NewPostgresWithContext(ctx context.Context, conf *configs.Configs) (*Postgres, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		conf.DataBase.Postgres.Host,
		conf.DataBase.Postgres.Port,
		conf.DataBase.Postgres.User,
		conf.DataBase.Postgres.Password,
		conf.DataBase.Postgres.Database,
		conf.DataBase.Postgres.SSLMode,
	)

	connConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	//if conf.Env == "local" {
	//	slogLogger := log.SlogJSON()
	//
	//	// integration opentelemetry
	//	tracer, err := dbtracer.NewDBTracer(
	//		conf.PostgresDBName,
	//		dbtracer.WithLogger(slogLogger),
	//		dbtracer.WithTraceProvider(otel.GetTracerProvider()),
	//		dbtracer.WithMeterProvider(metrics.MeterProvider()),
	//		dbtracer.WithLogArgs(false),
	//		dbtracer.WithIncludeSQLText(false),
	//		dbtracer.WithLogArgsLenLimit(1000),
	//	)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	connConfig.ConnConfig.Tracer = tracer
	//}

	connConfig.MaxConns = conf.DataBase.Postgres.MaxConnections
	connConfig.MinConns = conf.DataBase.Postgres.MinConnections
	connConfig.MaxConnLifetime = conf.DataBase.Postgres.MaxConnLifetime
	connConfig.MaxConnIdleTime = conf.DataBase.Postgres.MaxConnIdleTime
	connConfig.HealthCheckPeriod = 15 * time.Second
	connConfig.ConnConfig.RuntimeParams["application_name"] = conf.ApplicationName

	pool, err := pgxpool.NewWithConfig(ctx, connConfig)
	if err != nil {
		return nil, err
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}

	return &Postgres{
		pool: pool,
	}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Pool() *pgxpool.Pool {
	return p.pool
}

func (p *Postgres) Exec(ctx context.Context, sql string, arguments ...any) (commandtag pgconn.CommandTag, err error) {
	return p.pool.Exec(ctx, sql, arguments...)
}

func (p *Postgres) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.pool.Query(ctx, sql, args...)
}

func (p *Postgres) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.pool.QueryRow(ctx, sql, args...)
}

func (p *Postgres) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return p.pool.SendBatch(ctx, b)
}
