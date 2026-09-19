//go:build integration

package nats_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	ledgernats "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const relaySubject = "ledger.transaction.created"

func TestOutboxRelayJetStreamIntegration(t *testing.T) {
	ctx := context.Background()
	container, js, closeJetStream := startJetStream(t, ctx)
	defer closeJetStream()
	defer testcontainers.TerminateContainer(container)

	t.Run("publishes and marks outbox successful", func(t *testing.T) {
		item, err := outbox.NewOutbox("transaction-id", []byte(`{"transaction_id":"transaction-id"}`))
		if err != nil {
			t.Fatal(err)
		}
		repository := &integrationRepository{items: []*outbox.Outbox{item}}
		relay := ledgernats.NewOutboxRelay(repository, js, integrationTracer{}, logger.NewLogger(), integrationMetrics{}, configs.OutboxRelay{
			BatchSize: 1, MaxWorkers: 1, MaxAttempts: 3,
		})

		runCtx, cancel := context.WithCancel(ctx)
		repository.updatedCh = make(chan outbox.UpdateOutboxData, 1)
		done := make(chan error, 1)
		go func() { done <- relay.Run(runCtx) }()
		select {
		case <-repository.updatedCh:
			cancel()
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("timed out waiting for relay publication")
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if repository.updated.Status != outbox.Success {
			t.Fatalf("outbox status = %q, want %q", repository.updated.Status, outbox.Success)
		}

		consumer, err := js.CreateOrUpdateConsumer(ctx, "LEDGER_INTEGRATION", jetstream.ConsumerConfig{
			FilterSubject: relaySubject,
			AckPolicy:     jetstream.AckExplicitPolicy,
		})
		if err != nil {
			t.Fatal(err)
		}
		message, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if string(message.Data()) != string(item.Payload) {
			t.Fatalf("payload = %s, want %s", message.Data(), item.Payload)
		}
		if err := message.Ack(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("redelivers after nak without changing financial state", func(t *testing.T) {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     "LEDGER_REDELIVERY",
			Subjects: []string{"ledger.integration.redelivery"},
			Storage:  jetstream.MemoryStorage,
		})
		if err != nil {
			t.Fatal(err)
		}
		consumer, err := js.CreateOrUpdateConsumer(ctx, "LEDGER_REDELIVERY", jetstream.ConsumerConfig{
			FilterSubject: "ledger.integration.redelivery",
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       100 * time.Millisecond,
			MaxDeliver:    2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := js.Publish(ctx, "ledger.integration.redelivery", []byte(`{"event_id":"same-event"}`)); err != nil {
			t.Fatal(err)
		}
		first, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if err := first.Nak(); err != nil {
			t.Fatal(err)
		}
		second, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		metadata, err := second.Metadata()
		if err != nil {
			t.Fatal(err)
		}
		if metadata.NumDelivered != 2 {
			t.Fatalf("delivery count = %d, want 2", metadata.NumDelivered)
		}
		if err := second.Ack(); err != nil {
			t.Fatal(err)
		}
	})
}

func startJetStream(t *testing.T, ctx context.Context) (testcontainers.Container, jetstream.JetStream, func()) {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:2.10-alpine",
			Cmd:          []string{"-js", "-p", "4222"},
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForListeningPort("4222/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "4222/tcp")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := nats.Connect(fmt.Sprintf("nats://%s:%s", host, port.Port()))
	if err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: relayStreamName(), Subjects: []string{relaySubject}, Storage: jetstream.MemoryStorage,
	}); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	return container, js, connection.Close
}

func relayStreamName() string { return "LEDGER_INTEGRATION" }

type integrationRepository struct {
	items     []*outbox.Outbox
	updated   outbox.UpdateOutboxData
	updatedCh chan outbox.UpdateOutboxData
}

func (r *integrationRepository) Save(context.Context, *outbox.Outbox) error { return nil }
func (r *integrationRepository) ClaimPending(context.Context, int, int, time.Duration) ([]*outbox.Outbox, error) {
	return r.items, nil
}
func (r *integrationRepository) FindAll(context.Context, outbox.StatusOutbox, int) ([]*outbox.Outbox, error) {
	return nil, nil
}
func (r *integrationRepository) UpdateOutboxData(_ context.Context, _ entity.ID, data outbox.UpdateOutboxData) error {
	r.updated = data
	if r.updatedCh != nil {
		r.updatedCh <- data
	}
	return nil
}

type integrationTracer struct{}

func (integrationTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, integrationSpan{}
}

type integrationSpan struct{}

func (integrationSpan) End()                                 {}
func (integrationSpan) SpanContext() application.SpanContext { return integrationSpanContext{} }
func (integrationSpan) RecordError(error)                    {}

type integrationSpanContext struct{}

func (integrationSpanContext) TraceID() string { return "integration-trace" }

type integrationMetrics struct{}

func (integrationMetrics) RecordRequestTotal(string, string, int)                {}
func (integrationMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (integrationMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (integrationMetrics) RecordTransactionTotal(string)                         {}
func (integrationMetrics) RecordCommandTotal(string, string)                     {}
func (integrationMetrics) RecordCommandDuration(string, float64)                 {}
func (integrationMetrics) RecordIdempotencyTotal(string)                         {}
func (integrationMetrics) RecordConcurrencyRetry()                               {}
func (integrationMetrics) RecordOutboxTotal(string, string)                      {}
