//go:build unit

package nats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	relayNATS "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type relayRepository struct {
	items      []*outbox.Outbox
	update     outbox.UpdateOutboxData
	claimCalls int
}

func (r *relayRepository) Save(context.Context, *outbox.Outbox) error { return nil }
func (r *relayRepository) ClaimPending(context.Context, int, int, time.Duration) ([]*outbox.Outbox, error) {
	r.claimCalls++
	return r.items, nil
}
func (r *relayRepository) FindAll(context.Context, outbox.StatusOutbox, int) ([]*outbox.Outbox, error) {
	return nil, nil
}
func (r *relayRepository) UpdateOutboxData(_ context.Context, _ entity.ID, data outbox.UpdateOutboxData) error {
	r.update = data
	return nil
}

type relayJetStream struct {
	msg       *nats.Msg
	err       error
	onPublish func(context.Context)
	calls     int
}

func (j *relayJetStream) PublishMsg(ctx context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.msg = msg
	j.calls++
	if j.onPublish != nil {
		j.onPublish(ctx)
	}
	return nil, j.err
}

type relayTracer struct{}

func (relayTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, relaySpan{}
}

type relaySpan struct{}

func (relaySpan) End()                                 {}
func (relaySpan) SpanContext() application.SpanContext { return relaySpanContext{} }
func (relaySpan) RecordError(error)                    {}

type relaySpanContext struct{}

func (relaySpanContext) TraceID() string { return "relay-test" }

type relayMetrics struct{}

func (relayMetrics) RecordRequestTotal(string, string, int)                {}
func (relayMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (relayMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (relayMetrics) RecordTransactionTotal(string)                         {}
func (relayMetrics) RecordCommandTotal(string, string)                     {}
func (relayMetrics) RecordCommandDuration(string, float64)                 {}
func (relayMetrics) RecordIdempotencyTotal(string)                         {}
func (relayMetrics) RecordConcurrencyRetry()                               {}
func (relayMetrics) RecordOutboxTotal(string, string)                      {}

var _ = Describe("INTERNAL :: INFRA :: NATS :: OUTBOX RELAY", func() {
	Describe("#PublishBatch", func() {
		Context("success cases", func() {
			It("should publish pending events with a deduplication header and mark them successful", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte(`{"transaction_id":"transaction-id"}`))
				Expect(err).NotTo(HaveOccurred())
				item.Attempts = 1
				repo := &relayRepository{items: []*outbox.Outbox{item}}
				js := &relayJetStream{}
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.PublishBatch(context.Background())

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(js.msg).NotTo(BeNil())
				Expect(js.msg.Header.Get("Nats-Msg-Id")).To(Equal(item.ID.String()))
				Expect(repo.update.Status).To(Equal(outbox.Success))
			})
		})

		Context("error cases", func() {
			It("should publish exhausted events to the dead-letter subject and mark them failed", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
				Expect(err).NotTo(HaveOccurred())
				item.Attempts = 3
				repo := &relayRepository{items: []*outbox.Outbox{item}}
				js := &relayJetStream{err: errors.New("nats unavailable")}
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.PublishBatch(context.Background())

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(js.msg).NotTo(BeNil())
				Expect(js.msg.Subject).To(Equal(string(item.EventType) + ".dlq"))
				Expect(repo.update.Status).To(Equal(outbox.Failed))
			})
		})
	})

	Describe("#Run", func() {
		Context("success cases", func() {
			It("should stop claiming batches after shutdown", func() {
				// Arrange (Given)
				item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
				Expect(err).NotTo(HaveOccurred())
				repo := &relayRepository{items: []*outbox.Outbox{item}}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				js := &relayJetStream{onPublish: func(context.Context) { cancel() }}
				relay := newRelayForTest(repo, js, 3)

				// Act (When)
				err = relay.Run(ctx)

				// Assert (Then)
				Expect(err).NotTo(HaveOccurred())
				Expect(repo.claimCalls).To(Equal(1))
				Expect(js.calls).To(Equal(1))
				Expect(repo.update.Status).To(Equal(outbox.Success))
			})
		})
	})
})

func newRelayForTest(repo *relayRepository, js *relayJetStream, maxAttempts int) *relayNATS.OutboxRelay {
	return relayNATS.NewOutboxRelay(repo, js, relayTracer{}, logger.NewLogger(), relayMetrics{}, configs.OutboxRelay{
		BatchSize: 10, MaxWorkers: 1, MaxAttempts: maxAttempts,
	})
}

func TestOutboxRelay(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NATS Outbox Relay Suite")
}
