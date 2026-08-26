package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/factory"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue/types"
	"github.com/andreis3/isura-ledger-ms/internal/util"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	DefaultRetryDelay = 5 * time.Second
	MaxBackoffDelay   = 30 * time.Second
	BackoffBase       = 2.0 // Base for exponential backoff calculation
)

type eventEnvelope struct {
	Type event.Type `json:"type"`
}

type NatsConsumerServer struct {
	dep        *dependency.BaseDeps
	publisher  event.Publisher
	maxWorkers int
	semaphore  chan struct{}
	handlers   map[event.Type]types.QueueHandler
}

func NewNatsConsumerServer(baseDeps *dependency.BaseDeps, publisher event.Publisher) *NatsConsumerServer {
	// Protection against MaxWorkers = 0 locking up the semaphore channel.
	maxW := baseDeps.Cfg.Nats.Consumer.MaxWorkers
	if maxW <= 0 {
		maxW = 1
	}

	return &NatsConsumerServer{
		dep:        baseDeps,
		publisher:  publisher,
		maxWorkers: maxW,
		semaphore:  make(chan struct{}, maxW),
		// Optimization: Handler cache
		handlers: map[event.Type]types.QueueHandler{
			event.CreatedBalance: factory.NewCreateBalanceFactory(baseDeps),
		},
	}
}

func (c *NatsConsumerServer) Start(ctx context.Context) error {
	start := time.Now()

	consumer, err := c.consumer(ctx)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		select {
		case c.semaphore <- struct{}{}:
			go func() {
				defer func() { <-c.semaphore }()

				// Context Isolation!
				// Prevents a SIGTERM signal from abruptly interrupting the query in Postgres.
				workerCtx := context.WithoutCancel(ctx)

				// Extracts the trace, injecting it into the workerCtx, which is immune to cancellation.
				msgCtx := otel.GetTextMapPropagator().Extract(workerCtx, propagation.HeaderCarrier(msg.Headers()))

				c.processJob(msgCtx, msg)
			}()
		case <-ctx.Done():
			_ = msg.Nak()
			return
		}
	})
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	c.dep.Log.InfoText("NATS consumer started",
		slog.String("startup_time", time.Since(start).String()),
		slog.Int("max_workers", c.maxWorkers),
	)

	// Graceful Shutdown
	<-ctx.Done()
	c.dep.Log.InfoText("Sinal de encerramento recebido. Drenando mensagens em voo...")

	// Drains instead of stalling against brute force.
	cc.Drain()

	// Waits for ongoing goroutines to return slots to the semaphore, with a safety timeout
	drainCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for i := 0; i < c.maxWorkers; i++ {
		select {
		case c.semaphore <- struct{}{}:
		case <-drainCtx.Done():
			c.dep.Log.WarnText("Timeout aguardando drenagem dos workers do NATS. Forçando saída.")
			break
		}
	}

	return nil
}

func (c *NatsConsumerServer) consumer(ctx context.Context) (jetstream.Consumer, error) {
	cfg := c.dep.Cfg
	return c.dep.Nats.JS.CreateOrUpdateConsumer(ctx, cfg.Nats.Consumer.Stream,
		jetstream.ConsumerConfig{
			Name:          cfg.Nats.Consumer.Name,
			Durable:       cfg.Nats.Consumer.Durable,
			FilterSubject: cfg.Nats.Subject,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       cfg.Nats.Consumer.AckWait * time.Second,
			MaxDeliver:    cfg.Nats.Consumer.MaxDeliver,
		})
}

func (c *NatsConsumerServer) processJob(ctx context.Context, msg jetstream.Msg) {

	workerCtx, span := c.dep.Tracer.Start(ctx, "NatsConsumerServer.ProcessWorker")
	defer span.End()

	err := c.dispatch(workerCtx, msg)
	if err != nil {

		span.RecordError(err)

		c.dep.Log.ErrorJSON("Failed to process event",
			"error", err.Error(),
			"subject", msg.Subject(),
		)

		metadata, metaErr := msg.Metadata()
		if metaErr != nil {
			c.dep.Log.ErrorJSON("Failed to get message metadata", "error", metaErr.Error())
		}

		var permErr *queue.PermanentError
		isPermanent := errors.As(err, &permErr)

		var transErr *queue.TransientError
		isTransient := errors.As(err, &transErr)

		maxDeliveries := uint64(c.dep.Cfg.Nats.Consumer.MaxDeliver)
		retriesExhausted := metaErr == nil && maxDeliveries > 0 && metadata.NumDelivered >= maxDeliveries

		// The DLQ path (Exhaustion, Permanent Error, or Explicit Request)
		if isPermanent || retriesExhausted || (isTransient && transErr.SendToDLQ) {
			reason := "permanent error"
			if retriesExhausted {
				reason = "max deliveries reached"
			}

			c.publishToDLQ(workerCtx, msg, err)
			_ = msg.TermWithReason(reason)
			return
		}

		// Transient Error: Recalculate Backoff
		delay := DefaultRetryDelay
		if isTransient {
			delay = transErr.Delay
		} else if metaErr == nil {
			delay = backoffDelay(metadata.NumDelivered)
		}

		_ = msg.NakWithDelay(delay)
		return
	}

	_ = msg.Ack()
}

func (c *NatsConsumerServer) publishToDLQ(ctx context.Context, msg jetstream.Msg, err error) {
	c.dep.Log.ErrorJSON("Message marked for DLQ, publishing to dead letter subject",
		"subject", msg.Subject(),
		"error", err.Error(),
	)

	dlqEvent := event.NewDeadLetterEvent(msg.Subject(), msg.Data())
	if pubErr := c.publisher.Publish(ctx, dlqEvent); pubErr != nil {
		c.dep.Log.ErrorJSON("Failed to publish message to Dead Letter Queue", "error", pubErr.Error())
	}
}

func (c *NatsConsumerServer) dispatch(ctx context.Context, msg jetstream.Msg) error {
	var envelope eventEnvelope
	if err := util.JsonEngine.Unmarshal(msg.Data(), &envelope); err != nil {
		// A malformed payload will never be fixed by a retry.
		return queue.NewPermanentError(fmt.Errorf("failed to unmarshal event envelope: %w", err))
	}

	handler, exists := c.handlers[envelope.Type]
	if !exists {
		// Fase 2: Poison Pill resolvido. Evento desconhecido aciona a DLQ direto.
		return queue.NewPermanentError(fmt.Errorf("unsupported event type: %s", envelope.Type))
	}

	return handler.Handle(ctx, msg)
}

func backoffDelay(numDelivered uint64) time.Duration {
	seconds := math.Pow(BackoffBase, float64(numDelivered))
	if seconds >= MaxBackoffDelay.Seconds() {
		return MaxBackoffDelay
	}
	return time.Duration(seconds * float64(time.Second))
}
