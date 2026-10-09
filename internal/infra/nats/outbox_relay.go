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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// JetStreamPublisher publishes messages through a JetStream context.
type JetStreamPublisher interface {
	PublishMsg(context.Context, *nats.Msg, ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// OutboxRelay publishes already-committed outbox records. ClaimPending marks
// a record as in-flight before this component calls NATS, so a process crash
// leaves a retryable FAILED record instead of holding a database lock.
type OutboxRelay struct {
	repository outbox.Repository
	jetstream  JetStreamPublisher
	tracer     application.Tracer
	log        application.Logger
	metrics    application.Metrics
	config     configs.OutboxRelay
	workers    int
}

func NewOutboxRelay(repository outbox.Repository, js JetStreamPublisher, tracer application.Tracer, log application.Logger, metrics application.Metrics, config configs.OutboxRelay) *OutboxRelay {
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
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 10 * time.Second
	}
	workers := config.MaxWorkers
	if workers <= 0 {
		workers = 1
	}
	return &OutboxRelay{repository: repository, jetstream: js, tracer: tracer, log: log, metrics: metrics, config: config, workers: workers}
}

// Run polls until ctx is canceled. A claimed batch gets a bounded drain period
// on shutdown, after which active publish operations are canceled and awaited.
func (r *OutboxRelay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := r.PublishBatch(ctx); err != nil && ctx.Err() == nil {
			r.log.ErrorJSON("outbox relay batch failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// PublishBatch claims and publishes one batch of pending outbox records.
func (r *OutboxRelay) PublishBatch(ctx context.Context) error {
	items, err := r.repository.ClaimPending(ctx, r.config.BatchSize, r.config.MaxAttempts, r.config.RetryAfter)
	if err != nil {
		return fmt.Errorf("claim pending outbox: %w", err)
	}
	if len(items) == 0 {
		return nil
	}

	batchCtx, finishBatch := r.batchContext(ctx)
	defer finishBatch()

	sem := make(chan struct{}, r.workers)
	var wg sync.WaitGroup
itemsLoop:
	for index, item := range items {
		item := item
		if batchCtx.Err() != nil {
			r.releaseUnstarted(items[index:])
			break itemsLoop
		}
		select {
		case sem <- struct{}{}:
		case <-batchCtx.Done():
			r.releaseUnstarted(items[index:])
			break itemsLoop
		}
		if batchCtx.Err() != nil {
			<-sem
			r.releaseUnstarted(items[index:])
			break itemsLoop
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.publishOne(batchCtx, item)
		}()
	}
	wg.Wait()
	return nil
}

func (r *OutboxRelay) batchContext(ctx context.Context) (context.Context, func()) {
	batchCtx, cancelBatch := context.WithCancel(context.WithoutCancel(ctx))
	batchDone := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		select {
		case <-ctx.Done():
			timer := time.NewTimer(r.config.ShutdownTimeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				cancelBatch()
			case <-batchDone:
			}
		case <-batchDone:
		}
	}()
	return batchCtx, func() {
		close(batchDone)
		cancelBatch()
		<-drainDone
	}
}

func (r *OutboxRelay) publishOne(ctx context.Context, item *outbox.Outbox) {
	workerCtx, span := r.tracer.Start(ctx, "OutboxRelay.Publish")
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
		if ctx.Err() != nil {
			r.releaseForRetry(item, err)
		} else {
			r.handleFailure(workerCtx, item, err)
		}
	} else {
		r.metrics.RecordOutboxTotal(string(outbox.Success), string(item.EventType))
		r.metrics.RecordCommandTotal("OutboxRelay", "published")
		r.log.InfoJSON("outbox event published", slog.String("outbox_id", item.ID.String()), slog.String("subject", msg.Subject))
	}
	r.metrics.RecordCommandDuration("OutboxRelay", float64(time.Since(start).Milliseconds()))
}

// releaseForRetry restores the attempt consumed by ClaimPending when shutdown
// cancels an in-flight publish. ClaimPending already left the record FAILED,
// so a failed cleanup still leaves it recoverable when attempts remain.
func (r *OutboxRelay) releaseForRetry(item *outbox.Outbox, publishErr error) {
	r.log.ErrorJSON("outbox publication canceled during shutdown", slog.String("outbox_id", item.ID.String()), slog.String("error", publishErr.Error()))
	if item.Attempts <= 0 {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.restoreAttempt(cleanupCtx, item)
	r.metrics.RecordCommandTotal("OutboxRelay", "failed")
	r.metrics.RecordOutboxTotal(string(outbox.Failed), string(item.EventType))
}

func (r *OutboxRelay) releaseUnstarted(items []*outbox.Outbox) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, item := range items {
		if cleanupCtx.Err() != nil {
			return
		}
		r.restoreAttempt(cleanupCtx, item)
	}
}

func (r *OutboxRelay) restoreAttempt(ctx context.Context, item *outbox.Outbox) {
	if item.Attempts <= 0 {
		return
	}
	if err := r.repository.UpdateOutboxData(ctx, item.ID, outbox.UpdateOutboxData{
		Status: outbox.Failed, Attempts: item.Attempts - 1, LastAttemptAt: item.LastAttemptAt,
	}); err != nil {
		r.log.ErrorJSON("outbox retry state restoration failed", slog.String("outbox_id", item.ID.String()), slog.String("error", err.Error()))
	}
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
