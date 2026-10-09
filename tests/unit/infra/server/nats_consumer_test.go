//go:build unit

package server_test

import (
	"context"
	"errors"
	"sync"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	"github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/andreis3/isura-ledger-ms/internal/infra/server"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: SERVER :: NATS CONSUMER", func() {
	Describe("#Start", func() {
		Context("success cases", func() {
			It("should wait for in-flight processing before returning from shutdown", func() {
				// Arrange (Given)
				handlerEntered := make(chan struct{})
				releaseHandler := make(chan struct{})
				consumeContext := &fakeConsumeContext{closed: make(chan struct{}), drained: make(chan struct{})}
				message := &blockingHeadersMessage{
					headersEntered: handlerEntered,
					releaseHeaders: releaseHandler,
				}
				consumer := &fakeConsumer{
					consumeContext: consumeContext,
					message:        message,
				}
				baseDeps := &dependency.BaseDeps{
					Cfg:    &configs.Configs{Nats: configs.Nats{Consumer: configs.NatsConsumer{Stream: "ledger", MaxDeliver: 5}}},
					Log:    logger.NewLogger(),
					Tracer: adaptermocks.SilentTracerMock{},
					Nats:   &nats.ClientNats{JS: &fakeJetStream{consumer: consumer}},
				}
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				startResult := make(chan error, 1)

				// Act (When)
				go func() { startResult <- sut.Start(ctx) }()
				<-handlerEntered
				cancel()
				<-consumeContext.drained

				// Assert (Then)
				select {
				case <-startResult:
					Fail("Start returned while a worker was still processing")
				default:
				}
				close(releaseHandler)
				Expect(<-startResult).NotTo(HaveOccurred())
				Expect(message.terminated).To(BeTrue())
			})

			It("should cancel in-flight processing after the drain timeout and wait for it to exit", func() {
				// Arrange (Given)
				tracerEntered := make(chan struct{})
				consumeContext := &fakeConsumeContext{closed: make(chan struct{}), drained: make(chan struct{})}
				consumer := &fakeConsumer{
					consumeContext: consumeContext,
					message:        &immediateMessage{},
				}
				baseDeps := &dependency.BaseDeps{
					Cfg:    &configs.Configs{Nats: configs.Nats{Consumer: configs.NatsConsumer{Stream: "ledger", MaxDeliver: 5}}},
					Log:    logger.NewLogger(),
					Tracer: blockingTracer{entered: tracerEntered},
					Nats:   &nats.ClientNats{JS: &fakeJetStream{consumer: consumer}},
				}
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})
				ctx, cancel := context.WithCancel(context.Background())
				startResult := make(chan error, 1)

				// Act (When)
				go func() { startResult <- sut.Start(ctx) }()
				<-tracerEntered
				cancel()
				<-consumeContext.drained

				// Assert (Then)
				Expect(<-startResult).NotTo(HaveOccurred())
			})

			It("should negatively acknowledge messages received after cancellation", func() {
				// Arrange (Given)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				message := &immediateMessage{}
				consumeContext := &fakeConsumeContext{closed: make(chan struct{}), drained: make(chan struct{})}
				consumer := &fakeConsumer{
					consumeContext: consumeContext,
					message:        message,
					beforeConsume:  func() { cancel() },
				}
				baseDeps := baseDependencies(&fakeJetStream{consumer: consumer})
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})

				// Act (When)
				err := sut.Start(ctx)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(message.nakCount).To(Equal(1))
			})
		})

		Context("error cases", func() {
			It("should return an error when consumer creation fails", func() {
				// Arrange (Given)
				baseDeps := baseDependencies(&fakeJetStream{err: errors.New("consumer unavailable")})
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})

				// Act (When)
				err := sut.Start(context.Background())

				// Assert (Then)
				Expect(err).To(MatchError(ContainSubstring("failed to create consumer")))
			})

			It("should return an error when starting the JetStream consumer fails", func() {
				// Arrange (Given)
				baseDeps := baseDependencies(&fakeJetStream{consumer: &fakeConsumer{err: errors.New("consume failed")}})
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})

				// Act (When)
				err := sut.Start(context.Background())

				// Assert (Then)
				Expect(err).To(MatchError(ContainSubstring("failed to start consuming")))
			})

			It("should continue draining when a message negative acknowledgment fails", func() {
				// Arrange (Given)
				message := &immediateMessage{nakErr: errors.New("acknowledgment unavailable")}
				consumeContext := &fakeConsumeContext{closed: make(chan struct{}), drained: make(chan struct{})}
				consumer := &fakeConsumer{consumeContext: consumeContext, message: message}
				baseDeps := baseDependencies(&fakeJetStream{consumer: consumer})
				sut := server.NewNatsConsumerServer(baseDeps, fakePublisher{})
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				// Act (When)
				err := sut.Start(ctx)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(message.nakCount).To(Equal(1))
			})
		})
	})
})

func baseDependencies(js *fakeJetStream) *dependency.BaseDeps {
	return &dependency.BaseDeps{
		Cfg:    &configs.Configs{Nats: configs.Nats{Consumer: configs.NatsConsumer{Stream: "ledger", MaxDeliver: 5}}},
		Log:    logger.NewLogger(),
		Tracer: adaptermocks.SilentTracerMock{},
		Nats:   &nats.ClientNats{JS: js},
	}
}

type fakeJetStream struct {
	jetstream.JetStream
	consumer jetstream.Consumer
	err      error
}

func (f *fakeJetStream) CreateOrUpdateConsumer(context.Context, string, jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return f.consumer, f.err
}

type fakeConsumer struct {
	jetstream.Consumer
	consumeContext jetstream.ConsumeContext
	message        jetstream.Msg
	err            error
	beforeConsume  func()
}

func (f *fakeConsumer) Consume(handler jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if f.beforeConsume != nil {
		f.beforeConsume()
	}
	if f.err != nil {
		return nil, f.err
	}
	handler(f.message)
	return f.consumeContext, nil
}

type fakeConsumeContext struct {
	closed  chan struct{}
	drained chan struct{}
	once    sync.Once
}

func (f *fakeConsumeContext) Stop() { f.Drain() }

func (f *fakeConsumeContext) Drain() {
	f.once.Do(func() {
		close(f.drained)
		close(f.closed)
	})
}

func (f *fakeConsumeContext) Closed() <-chan struct{} { return f.closed }

type blockingHeadersMessage struct {
	jetstream.Msg
	headersEntered chan struct{}
	releaseHeaders chan struct{}
	terminated     bool
}

func (m *blockingHeadersMessage) Headers() natsgo.Header {
	close(m.headersEntered)
	<-m.releaseHeaders
	return nil
}

func (*blockingHeadersMessage) Data() []byte { return []byte("invalid json") }

func (*blockingHeadersMessage) Subject() string { return "ledger.events" }

func (*blockingHeadersMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}

func (m *blockingHeadersMessage) TermWithReason(string) error {
	m.terminated = true
	return nil
}

type immediateMessage struct {
	jetstream.Msg
	nakCount int
	nakErr   error
}

func (*immediateMessage) Headers() natsgo.Header { return nil }

func (*immediateMessage) Data() []byte { return []byte("invalid json") }

func (*immediateMessage) Subject() string { return "ledger.events" }

func (*immediateMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}

func (m *immediateMessage) Nak() error {
	m.nakCount++
	return m.nakErr
}

func (*immediateMessage) TermWithReason(string) error { return nil }

type fakePublisher struct{}

func (fakePublisher) Publish(context.Context, event.Event) error { return nil }

type blockingTracer struct {
	entered chan struct{}
}

func (t blockingTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	close(t.entered)
	<-ctx.Done()
	return ctx, testSpan{}
}

type testSpan struct{}

func (testSpan) End()                                 {}
func (testSpan) SpanContext() application.SpanContext { return testSpanContext{} }
func (testSpan) RecordError(error)                    {}

type testSpanContext struct{}

func (testSpanContext) TraceID() string { return "unit-test-trace" }

var _ jetstream.JetStream = (*fakeJetStream)(nil)
var _ jetstream.Consumer = (*fakeConsumer)(nil)
var _ jetstream.ConsumeContext = (*fakeConsumeContext)(nil)
var _ jetstream.Msg = (*blockingHeadersMessage)(nil)
var _ jetstream.Msg = (*immediateMessage)(nil)
var _ event.Publisher = fakePublisher{}
var _ application.Tracer = blockingTracer{}
var _ application.Span = testSpan{}
