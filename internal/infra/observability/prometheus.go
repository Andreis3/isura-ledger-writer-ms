package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
	MeterName    = "isura-ledger-ms"
	MeterVersion = "1.0.0"
)

type Prometheus struct {
	provider                              *metric.MeterProvider
	ledgerRequestsTotal                   api.Int64Counter
	ledgerDbQueryDurationMilliseconds     api.Float64Histogram
	ledgerGrpcRequestDurationMilliseconds api.Float64Histogram
	ledgerTransactionsTotal               api.Int64Counter // {status: completed|failed}
	ledgerCommandDurationMilliseconds     api.Float64Histogram
	ledgerCommandTotal                    api.Int64Counter
}

func NewPrometheus() (*Prometheus, error) {
	exporterInstance, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(MeterName),
		semconv.ServiceVersionKey.String(MeterVersion),
	)

	meterProviderInstance := metric.NewMeterProvider(metric.WithReader(exporterInstance), metric.WithResource(res))
	meter := meterProviderInstance.Meter(MeterName, api.WithInstrumentationVersion(MeterVersion))

	ledgerRequestsTotal, err := meter.Int64Counter("ledger_requests_total",
		api.WithDescription("Total number of requests by status code"))

	if err != nil {
		return nil, err
	}

	ledgerDbQueryDurationMilliseconds, err := meter.Float64Histogram("ledger_db_query_duration_milliseconds",
		api.WithDescription("Histogram of instruction duration"),
		api.WithExplicitBucketBoundaries(
			5, 10, 15, 20, 30, 50,
			100, 200, 300, 500, 1000,
			2000, 5000, 10000, 20000,
			30000, 50000, 100000))
	if err != nil {
		return nil, err
	}

	ledgerGrpcRequestDurationMilliseconds, err := meter.Float64Histogram("ledger_grpc_request_duration_milliseconds",
		api.WithDescription("Histogram of request duration"),
		api.WithExplicitBucketBoundaries(
			5, 10, 15, 20, 30, 50,
			100, 200, 300, 500, 1000,
			2000, 5000, 10000, 20000,
			30000, 50000, 100000))
	if err != nil {
		return nil, err
	}

	ledgerCommandDurationMilliseconds, err := meter.Float64Histogram("ledger_command_duration_milliseconds",
		api.WithDescription("Histogram of command duration"),
		api.WithExplicitBucketBoundaries(
			5, 10, 15, 20, 30, 50,
			100, 200, 300, 500, 1000,
			2000, 5000, 10000, 20000,
			30000, 50000, 100000))
	if err != nil {
		return nil, err
	}

	ledgerTransactionsTotal, err := meter.Int64Counter("ledger_transactions_total",
		api.WithDescription("Total number of transactions by status"))
	if err != nil {
		return nil, err
	}

	ledgerCommandTotal, err := meter.Int64Counter("ledger_command_total",
		api.WithDescription("Total number of commands by status"),
	)
	if err != nil {
		return nil, err
	}

	return &Prometheus{
		provider:                              meterProviderInstance,
		ledgerRequestsTotal:                   ledgerRequestsTotal,
		ledgerDbQueryDurationMilliseconds:     ledgerDbQueryDurationMilliseconds,
		ledgerGrpcRequestDurationMilliseconds: ledgerGrpcRequestDurationMilliseconds,
		ledgerTransactionsTotal:               ledgerTransactionsTotal,
		ledgerCommandDurationMilliseconds:     ledgerCommandDurationMilliseconds,
		ledgerCommandTotal:                    ledgerCommandTotal,
	}, nil
}

func (p *Prometheus) RecordRequestTotal(router, protocol string, statusCode int) {
	opt := api.WithAttributes(
		attribute.Key("router").String(router),
		attribute.Key("status_code").Int(statusCode),
		attribute.Key("protocol").String(protocol),
	)
	p.ledgerRequestsTotal.Add(context.Background(), 1, opt)
}

func (p *Prometheus) RecordDBQueryDuration(database, table, method string, duration float64) {
	opt := api.WithAttributes(
		attribute.Key("database").String(database),
		attribute.Key("table").String(table),
		attribute.Key("method").String(method),
	)
	p.ledgerDbQueryDurationMilliseconds.Record(context.Background(), duration, opt)
}

func (p *Prometheus) RecordRequestDuration(router, protocol string, statusCode int, duration float64) {
	opt := api.WithAttributes(
		attribute.Key("router").String(router),
		attribute.Key("status_code").Int(statusCode),
		attribute.Key("protocol").String(protocol),
	)
	p.ledgerGrpcRequestDurationMilliseconds.Record(context.Background(), duration, opt)
}

func (p *Prometheus) RecordTransactionTotal(status string) {
	opt := api.WithAttributes(
		attribute.Key("status").String(status),
	)
	p.ledgerTransactionsTotal.Add(context.Background(), 1, opt)
}

func (p *Prometheus) RecordCommandTotal(command string, state string) {
	opt := api.WithAttributes(
		attribute.Key("command").String(command),
		attribute.Key("state").String(state))
	p.ledgerCommandTotal.Add(context.Background(), 1, opt)
}

func (p *Prometheus) RecordCommandDuration(command string, duration float64) {
	opt := api.WithAttributes(
		attribute.Key("command").String(command))
	p.ledgerCommandDurationMilliseconds.Record(context.Background(), duration, opt)
}

func (p *Prometheus) Close() {
	_ = p.provider.Shutdown(context.Background())
}

func (p *Prometheus) MeterProvider() *metric.MeterProvider {
	return p.provider
}
