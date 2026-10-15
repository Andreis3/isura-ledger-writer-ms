//go:build unit

package observability_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/observability"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: OBSERVABILITY :: METRICS", func() {
	Describe("#NewPrometheus", func() {
		Context("success cases", func() {
			It("should record supported ledger metrics and expose its provider", func() {
				// Arrange.
				sut, err := observability.NewPrometheus()
				Expect(err).NotTo(HaveOccurred())

				// Act.
				sut.RecordRequestTotal("/transactions", "rest", 201)
				sut.RecordDBQueryDuration("postgres", "transactions", "insert", 2.5)
				sut.RecordRequestDuration("/ledger", "grpc", 0, 4)
				sut.RecordTransactionTotal("completed")
				sut.RecordCommandTotal("create_transaction", "success")
				sut.RecordCommandDuration("create_transaction", 3)
				sut.RecordIdempotencyTotal("replay")
				sut.RecordConcurrencyRetry()
				sut.RecordOutboxTotal("published", "transaction.created")
				Expect(sut.MeterProvider()).NotTo(BeNil())

				// Assert.
				Expect(sut.MeterProvider().ForceFlush(context.Background())).To(Succeed())
				sut.Close()
			})
		})
	})
})

var _ = Describe("INTERNAL :: INFRA :: OBSERVABILITY :: OTEL TRACER", func() {
	Describe("#InitOtelTracer", func() {
		Context("success cases", func() {
			It("should create, record, and export a sampled span", func() {
				// Arrange.
				requests := 0
				collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					Expect(r.URL.Path).To(Equal("/v1/traces"))
					w.WriteHeader(http.StatusOK)
				}))
				DeferCleanup(collector.Close)
				endpoint := strings.TrimPrefix(collector.URL, "http://")
				cfg := &configs.Configs{ApplicationName: "unit-test-ledger", Version: "test", Env: "test", OpenTelemetry: configs.OpemTelemetry{Host: endpoint}}

				// Act.
				sut, shutdown, err := observability.InitOtelTracer(context.Background(), cfg)
				Expect(err).NotTo(HaveOccurred())
				ctx, span := sut.Start(context.Background(), "test.operation")
				Expect(ctx).NotTo(BeNil())
				Expect(span.SpanContext().TraceID()).NotTo(BeEmpty())
				span.RecordError(errors.New("operation failed"))
				span.End()
				err = shutdown(context.Background())

				// Assert.
				Expect(err).NotTo(HaveOccurred())
				Expect(requests).To(BeNumerically(">", 0))
			})
		})
	})
})
