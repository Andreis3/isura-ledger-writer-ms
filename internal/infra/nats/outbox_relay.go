package nats

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const relayShutdownTimeout = 15 * time.Second

type outboxJetStream interface {
	PublishMsg(context.Context, *nats.Msg, ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// OutboxRelay publishes already-committed outbox records. ClaimPending marks
// a record as in-flight before this component calls NATS, so a process crash
// leaves a retryable FAILED record instead of holding a database lock.
type OutboxRelay struct {
	repository outbox.Repository
	jetstream  outboxJetStream
	tracer     application.Tracer
	log        *logger.Logger
	metrics    application.Metrics
	config     configs.OutboxRelay
	workers    int
}

func NewOutboxRelay(repository outbox.Repository, js outboxJetStream, tracer application.Tracer, log *logger.Logger, metrics application.Metrics, config configs.OutboxRelay) *OutboxRelay {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = outbox.MaxAttempts
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.RetryAfter <= 0 {
		config.RetryAfter = 5 * time.Second
	}
	workers := config.MaxWorkers
	if workers <= 0 {
		workers = 1
	}
	return &OutboxRelay{repository: repository, jetstream: js, tracer: tracer, log: log, metrics: metrics, config: config, workers: workers}
}

// Run polls until ctx is canceled and drains publications already claimed.
func (r *OutboxRelay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := r.publishBatch(ctx); err != nil && ctx.Err() == nil {
			r.log.ErrorJSON("outbox relay batch failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			r.drain(ctx)
			return nil
		case <-ticker.C:
		}
	}
}

func (r *OutboxRelay) publishBatch(ctx context.Context) error {
	items, err := r.repository.ClaimPending(ctx, r.config.BatchSize, r.config.MaxAttempts, r.config.RetryAfter)
	if err != nil {
		return fmt.Errorf("claim pending outbox: %w", err)
	}
	if len(items) == 0 {
		return nil
	}

	sem := make(chan struct{}, r.workers)
	var wg sync.WaitGroup
	for _, item := range items {
		item := item
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.publishOne(ctx, item)
		}()
	}
	wg.Wait()
	return nil
}

func (r *OutboxRelay) publishOne(ctx context.Context, item *outbox.Outbox) {
	workerCtx := context.WithoutCancel(ctx)
	workerCtx, span := r.tracer.Start(workerCtx, "OutboxRelay.Publish")
	defer span.End()
	start := time.Now()

	msg := &nats.Msg{Subject: string(item.EventType), Data: item.Payload, Header: nats.Header{}}
	msg.Header.Set("Nats-Msg-Id", item.ID.String())
	otel.GetTextMapPropagator().Inject(workerCtx, propagation.HeaderCarrier(msg.Header))

	_, err := r.jetstream.PublishMsg(workerCtx, msg)
	if err == nil {
		now := time.Now()
		err = r.repository.UpdateOutboxData(workerCtx, item.ID, outbox.UpdateOutboxData{
			Status: outbox.Success, Attempts: item.Attempts, LastAttemptAt: item.LastAttemptAt, PublishedAt: &now,
		})
	}
	if err != nil {
		span.RecordError(err)
		r.handleFailure(workerCtx, item, err)
	} else {
		r.metrics.RecordOutboxTotal(string(outbox.Success), string(item.EventType))
		r.metrics.RecordCommandTotal("OutboxRelay", "published")
		r.log.InfoJSON("outbox event published", slog.String("outbox_id", item.ID.String()), slog.String("subject", msg.Subject))
	}
	r.metrics.RecordCommandDuration("OutboxRelay", float64(time.Since(start).Milliseconds()))
}

func (r *OutboxRelay) handleFailure(ctx context.Context, item *outbox.Outbox, publishErr error) {
	r.log.ErrorJSON("outbox event publication failed", slog.String("outbox_id", item.ID.String()), slog.String("error", publishErr.Error()))
	if item.Attempts >= r.config.MaxAttempts {
		if err := r.publishDLQ(ctx, item); err != nil {
			r.log.ErrorJSON("outbox dead-letter publication failed", slog.String("outbox_id", item.ID.String()), slog.String("error", err.Error()))
		}
	}
	if err := r.repository.UpdateOutboxData(ctx, item.ID, outbox.UpdateOutboxData{Status: outbox.Failed, Attempts: item.Attempts, LastAttemptAt: item.LastAttemptAt}); err != nil {
		r.log.ErrorJSON("outbox failure state update failed", slog.String("outbox_id", item.ID.String()), slog.String("error", err.Error()))
	}
	r.metrics.RecordCommandTotal("OutboxRelay", "failed")
	r.metrics.RecordOutboxTotal(string(outbox.Failed), string(item.EventType))
}

func (r *OutboxRelay) publishDLQ(ctx context.Context, item *outbox.Outbox) error {
	subject := r.config.DLQSubject
	if subject == "" {
		subject = string(item.EventType) + event.DLQSubjectSuffix
	}
	msg := &nats.Msg{Subject: subject, Data: item.Payload, Header: nats.Header{}}
	msg.Header.Set("Nats-Msg-Id", item.ID.String()+".dlq")
	_, err := r.jetstream.PublishMsg(ctx, msg)
	return err
}

func (r *OutboxRelay) drain(ctx context.Context) {
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), relayShutdownTimeout)
	defer cancel()
	if _, err := r.repository.ClaimPending(drainCtx, r.config.BatchSize, r.config.MaxAttempts, r.config.RetryAfter); err != nil && drainCtx.Err() == nil {
		r.log.WarnJSON("outbox relay drain claim failed", slog.String("error", err.Error()))
	}
}
