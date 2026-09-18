//go:build unit
// +build unit

package nats

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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type relayRepository struct {
	items  []*outbox.Outbox
	update outbox.UpdateOutboxData
}

func (r *relayRepository) Save(context.Context, *outbox.Outbox) error { return nil }
func (r *relayRepository) ClaimPending(context.Context, int, int, time.Duration) ([]*outbox.Outbox, error) {
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
	msg *nats.Msg
	err error
}

func (j *relayJetStream) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.msg = msg
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

func TestOutboxRelayPublishesWithDeduplicationHeaderAndMarksSuccess(t *testing.T) {
	item, err := outbox.NewOutbox("transaction-id", []byte(`{"transaction_id":"transaction-id"}`))
	if err != nil {
		t.Fatal(err)
	}
	item.Attempts = 1
	repo := &relayRepository{items: []*outbox.Outbox{item}}
	js := &relayJetStream{}
	relay := newRelayForTest(repo, js, 3)

	if err := relay.publishBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if js.msg == nil {
		t.Fatal("expected publication")
	}
	if got := js.msg.Header.Get("Nats-Msg-Id"); got != item.ID.String() {
		t.Fatalf("Nats-Msg-Id = %q, want %q", got, item.ID.String())
	}
	if repo.update.Status != outbox.Success {
		t.Fatalf("status = %q, want %q", repo.update.Status, outbox.Success)
	}
}

func TestOutboxRelayPublishesDLQAfterMaxAttempts(t *testing.T) {
	item, err := outbox.NewOutbox("transaction-id", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	item.Attempts = 3
	repo := &relayRepository{items: []*outbox.Outbox{item}}
	js := &relayJetStream{err: errors.New("nats unavailable")}
	relay := newRelayForTest(repo, js, 3)

	if err := relay.publishBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if js.msg == nil || js.msg.Subject != string(item.EventType)+".dlq" {
		t.Fatalf("expected DLQ publication, got %#v", js.msg)
	}
	if repo.update.Status != outbox.Failed {
		t.Fatalf("status = %q, want %q", repo.update.Status, outbox.Failed)
	}
}

func newRelayForTest(repo *relayRepository, js *relayJetStream, maxAttempts int) *OutboxRelay {
	return NewOutboxRelay(repo, js, relayTracer{}, logger.NewLogger(), relayMetrics{}, configs.OutboxRelay{
		BatchSize: 10, MaxWorkers: 1, MaxAttempts: maxAttempts,
	})
}
