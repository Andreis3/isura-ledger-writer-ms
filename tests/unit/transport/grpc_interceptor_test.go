//go:build unit

package transport_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/transport/grpc/interceptor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type interceptorMetricsFake struct {
	requestCount  int
	durationCount int
	protocol      string
	statusCode    int
}

func (f *interceptorMetricsFake) RecordRequestTotal(_, protocol string, statusCode int) {
	f.requestCount++
	f.protocol, f.statusCode = protocol, statusCode
}
func (*interceptorMetricsFake) RecordDBQueryDuration(string, string, string, float64) {}
func (f *interceptorMetricsFake) RecordRequestDuration(_, protocol string, statusCode int, _ float64) {
	f.durationCount++
	f.protocol, f.statusCode = protocol, statusCode
}
func (*interceptorMetricsFake) RecordTransactionTotal(string)         {}
func (*interceptorMetricsFake) RecordCommandTotal(string, string)     {}
func (*interceptorMetricsFake) RecordCommandDuration(string, float64) {}
func (*interceptorMetricsFake) RecordIdempotencyTotal(string)         {}
func (*interceptorMetricsFake) RecordConcurrencyRetry()               {}
func (*interceptorMetricsFake) RecordOutboxTotal(string, string)      {}

type interceptorSpanFake struct {
	ended       bool
	recordedErr error
}

func (f *interceptorSpanFake) End() { f.ended = true }
func (f *interceptorSpanFake) SpanContext() application.SpanContext {
	return interceptorSpanContextFake{}
}
func (f *interceptorSpanFake) RecordError(err error) { f.recordedErr = err }

type interceptorSpanContextFake struct{}

func (interceptorSpanContextFake) TraceID() string { return "trace" }

type interceptorTracerFake struct{ span *interceptorSpanFake }

func (f interceptorTracerFake) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, f.span
}

var _ application.Metrics = (*interceptorMetricsFake)(nil)
var _ application.Tracer = interceptorTracerFake{}

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: INTERCEPTOR", func() {
	Describe("#LoggingInterceptor", func() {
		Context("success cases", func() {
			It("should log successful, rejected, and failed calls at their matching levels", func() {
				// Arrange.
				var output bytes.Buffer
				log := slog.New(slog.NewTextHandler(&output, nil))
				calls := []struct {
					err error
					msg string
				}{
					{nil, "INFO"},
					{status.Error(codes.InvalidArgument, "bad request"), "WARN"},
					{status.Error(codes.Internal, "failed"), "ERROR"},
				}
				sut := interceptor.LoggingInterceptor(log)

				// Act.
				for _, call := range calls {
					response, err := sut(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.Service/Call"}, func(context.Context, any) (any, error) {
						return "response", call.err
					})
					Expect(response).To(Equal("response"))
					if call.err == nil {
						Expect(err).NotTo(HaveOccurred())
					} else {
						Expect(err).To(MatchError(call.err))
					}
				}

				// Assert.
				for _, call := range calls {
					Expect(output.String()).To(ContainSubstring(call.msg))
				}
				Expect(output.String()).To(ContainSubstring("/ledger.v1.Service/Call"))
			})
		})
	})

	Describe("#MetricsInterceptor", func() {
		Context("success cases", func() {
			It("should record the request and duration with the resulting status", func() {
				// Arrange.
				metrics := &interceptorMetricsFake{}
				sut := interceptor.MetricsInterceptor(metrics)

				// Act.
				response, err := sut(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.Service/Call"}, func(context.Context, any) (any, error) {
					return nil, status.Error(codes.InvalidArgument, "bad request")
				})

				// Assert.
				Expect(response).To(BeNil())
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(metrics.requestCount).To(Equal(1))
				Expect(metrics.durationCount).To(Equal(1))
				Expect(metrics.protocol).To(Equal("grpc"))
				Expect(metrics.statusCode).To(Equal(int(codes.InvalidArgument)))
			})
		})
	})

	Describe("#TracingInterceptor", func() {
		Context("success cases", func() {
			It("should end the span and record handler errors", func() {
				// Arrange.
				span := &interceptorSpanFake{}
				sut := interceptor.TracingInterceptor(interceptorTracerFake{span: span})
				wantErr := errors.New("handler failed")

				// Act.
				_, err := sut(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.Service/Call"}, func(context.Context, any) (any, error) {
					return nil, wantErr
				})

				// Assert.
				Expect(err).To(MatchError(wantErr))
				Expect(span.ended).To(BeTrue())
				Expect(span.recordedErr).To(MatchError(wantErr))
			})
		})
	})
})
