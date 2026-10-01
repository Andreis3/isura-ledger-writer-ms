//go:build integration

package nats_test

import (
	"context"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	ledgernats "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const relaySubject = "ledger.transaction.created"

var _ = Describe("INTEGRATION :: INFRA :: NATS :: OUTBOX RELAY", func() {
	Describe("#Run", func() {
		Context("success cases", func() {
			It("should publish an outbox event to JetStream and mark it successful", func() {
				// Arrange (Given)
				ctx := context.Background()
				container, js, closeJetStream, err := startJetStream(ctx)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					Expect(testcontainers.TerminateContainer(container)).To(Succeed())
				})
				DeferCleanup(closeJetStream)

				item, err := outbox.NewOutbox("transaction-id", []byte(`{"transaction_id":"transaction-id"}`))
				Expect(err).NotTo(HaveOccurred())
				repo := &integrationRepository{items: []*outbox.Outbox{item}, updatedCh: make(chan outbox.UpdateOutboxData, 1)}
				relay := newIntegrationRelay(repo, js)
				runCtx, cancel := context.WithCancel(ctx)
				DeferCleanup(cancel)

				// Act (When)
				done := make(chan error, 1)
				go func() { done <- relay.Run(runCtx) }()
				select {
				case update := <-repo.updatedCh:
					Expect(update.Status).To(Equal(outbox.Success))
					cancel()
				case <-time.After(5 * time.Second):
					cancel()
					Fail("timed out waiting for relay publication")
				}
				runErr := <-done

				// Assert (Then)
				Expect(runErr).NotTo(HaveOccurred())
				consumer, err := js.CreateOrUpdateConsumer(ctx, relayStreamName(), jetstream.ConsumerConfig{
					FilterSubject: relaySubject,
					AckPolicy:     jetstream.AckExplicitPolicy,
				})
				Expect(err).NotTo(HaveOccurred())
				message, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(message.Data())).To(Equal(string(item.Payload)))
				Expect(message.Ack()).To(Succeed())
			})
		})
	})

	Describe("JetStream consumer acknowledgements", func() {
		Context("redelivery cases", func() {
			It("should redeliver a message after NAK without changing its payload", func() {
				// Arrange (Given)
				ctx := context.Background()
				container, js, closeJetStream, err := startJetStream(ctx)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					Expect(testcontainers.TerminateContainer(container)).To(Succeed())
				})
				DeferCleanup(closeJetStream)
				_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
					Name: "LEDGER_REDELIVERY", Subjects: []string{"ledger.integration.redelivery"}, Storage: jetstream.MemoryStorage,
				})
				Expect(err).NotTo(HaveOccurred())
				consumer, err := js.CreateOrUpdateConsumer(ctx, "LEDGER_REDELIVERY", jetstream.ConsumerConfig{
					FilterSubject: "ledger.integration.redelivery",
					AckPolicy:     jetstream.AckExplicitPolicy,
					AckWait:       100 * time.Millisecond,
					MaxDeliver:    2,
				})
				Expect(err).NotTo(HaveOccurred())
				payload := []byte(`{"event_id":"same-event"}`)
				_, err = js.Publish(ctx, "ledger.integration.redelivery", payload)
				Expect(err).NotTo(HaveOccurred())

				// Act (When)
				first, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
				Expect(err).NotTo(HaveOccurred())
				Expect(first.Nak()).To(Succeed())
				second, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
				Expect(err).NotTo(HaveOccurred())
				metadata, err := second.Metadata()
				Expect(err).NotTo(HaveOccurred())

				// Assert (Then)
				Expect(string(second.Data())).To(Equal(string(payload)))
				Expect(metadata.NumDelivered).To(Equal(uint64(2)))
				Expect(second.Ack()).To(Succeed())
			})
		})
	})
})

func startJetStream(ctx context.Context) (testcontainers.Container, jetstream.JetStream, func(), error) {
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
		return nil, nil, nil, fmt.Errorf("start NATS container: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, nil, nil, fmt.Errorf("get NATS container host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4222/tcp")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, nil, nil, fmt.Errorf("get NATS mapped port: %w", err)
	}
	connection, err := nats.Connect(fmt.Sprintf("nats://%s:%s", host, port.Port()))
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, nil, nil, fmt.Errorf("connect to NATS container: %w", err)
	}
	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, nil, nil, fmt.Errorf("create JetStream context: %w", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: relayStreamName(), Subjects: []string{relaySubject}, Storage: jetstream.MemoryStorage,
	}); err != nil {
		connection.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, nil, nil, fmt.Errorf("create JetStream stream: %w", err)
	}
	return container, js, connection.Close, nil
}

func relayStreamName() string { return "LEDGER_INTEGRATION" }

func newIntegrationRelay(repo *integrationRepository, js jetstream.JetStream) *ledgernats.OutboxRelay {
	return ledgernats.NewOutboxRelay(repo, js, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{}, configs.OutboxRelay{
		BatchSize: 1, MaxWorkers: 1, MaxAttempts: 3,
	})
}

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

var _ outbox.Repository = (*integrationRepository)(nil)
