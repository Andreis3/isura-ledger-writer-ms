package adapter

import (
	"context"
	"log/slog"

	"github.com/andreis3/isura-ledger-ms/internal/application"
)

type SilentLoggerMock struct{}

func (SilentLoggerMock) DebugJSON(string, ...any)               {}
func (SilentLoggerMock) InfoJSON(string, ...any)                {}
func (SilentLoggerMock) WarnJSON(string, ...any)                {}
func (SilentLoggerMock) ErrorJSON(string, ...any)               {}
func (SilentLoggerMock) CriticalJSON(string, ...any)            {}
func (SilentLoggerMock) DebugText(string, ...any)               {}
func (SilentLoggerMock) InfoText(string, ...any)                {}
func (SilentLoggerMock) WarnText(string, ...any)                {}
func (SilentLoggerMock) ErrorText(string, ...any)               {}
func (SilentLoggerMock) CriticalText(string, ...any)            {}
func (SilentLoggerMock) WithTrace(context.Context) *slog.Logger { return slog.Default() }
func (SilentLoggerMock) SlogJSON() *slog.Logger                 { return slog.Default() }
func (SilentLoggerMock) SlogText() *slog.Logger                 { return slog.Default() }

type SilentTracerMock struct{}

func (SilentTracerMock) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, silentSpanMock{}
}

type silentSpanMock struct{}

func (silentSpanMock) End()                                 {}
func (silentSpanMock) SpanContext() application.SpanContext { return silentSpanContextMock{} }
func (silentSpanMock) RecordError(error)                    {}

type silentSpanContextMock struct{}

func (silentSpanContextMock) TraceID() string { return "unit-test-trace" }

type SilentMetricsMock struct{}

func (SilentMetricsMock) RecordRequestTotal(string, string, int)                {}
func (SilentMetricsMock) RecordDBQueryDuration(string, string, string, float64) {}
func (SilentMetricsMock) RecordRequestDuration(string, string, int, float64)    {}
func (SilentMetricsMock) RecordTransactionTotal(string)                         {}
func (SilentMetricsMock) RecordCommandTotal(string, string)                     {}
func (SilentMetricsMock) RecordCommandDuration(string, float64)                 {}
func (SilentMetricsMock) RecordIdempotencyTotal(string)                         {}
func (SilentMetricsMock) RecordConcurrencyRetry()                               {}
func (SilentMetricsMock) RecordOutboxTotal(string, string)                      {}

var (
	_ application.Logger  = SilentLoggerMock{}
	_ application.Tracer  = SilentTracerMock{}
	_ application.Metrics = SilentMetricsMock{}
)
