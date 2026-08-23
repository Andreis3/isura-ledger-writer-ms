package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
)

type otelTracer struct {
	tracer trace.Tracer
}

func (o *otelTracer) Start(ctx context.Context, spanName string) (context.Context, application.Span) {
	ctx, s := o.tracer.Start(ctx, spanName)
	return ctx, &otelSpan{s}
}

type otelSpan struct {
	trace.Span
}

func (s *otelSpan) End() {
	s.Span.End()
}

func (s *otelSpan) RecordError(err error) {
	s.Span.RecordError(err)
}

func (s *otelSpan) SpanContext() application.SpanContext {
	return &otelSpanContext{s.Span.SpanContext()}
}

type otelSpanContext struct {
	trace.SpanContext
}

func (sc *otelSpanContext) TraceID() string {
	return sc.SpanContext.TraceID().String()
}

func InitOtelTracer(ctx context.Context, cfg *configs.Configs) (application.Tracer, func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(cfg.OpenTelemetry.Host),
		otlptracehttp.WithInsecure(),
		// Tip: Remove Gzip compression if the collector is on the same machine/internal network
		// or reduce CPU pressure/Huffman buffers if the load is extreme.
		// otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	resource, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceName(cfg.ApplicationName),
			semconv.ServiceVersion(cfg.Version),
		),
		sdkresource.WithProcess(),
		sdkresource.WithOS(),
		sdkresource.WithHost(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 💡 CRITICAL OPTIMIZATION: Also reduce sampling outside of production if running load tests
	// Use 1% (0.01) or 5% (0.05) for high-concurrency stress tests.
	sampleRate := 1.0 // Default for dev
	if cfg.Env == "dev" || cfg.Env == "staging" || cfg.Env == "load-test" {
		sampleRate = 0.05 // Collects only 5% of traces under high load
	}
	sampler := sdktrace.TraceIDRatioBased(sampleRate)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			// Fine-tune the batch processor to relieve event contention
			sdktrace.WithMaxQueueSize(4096),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(provider)

	// === ADICIONE ESTAS LINHAS AQUI ===
	// Define o propagador padrão do W3C (TraceContext + Baggage) globalmente
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &otelTracer{tracer: provider.Tracer(cfg.ApplicationName)}, provider.Shutdown, nil
}
