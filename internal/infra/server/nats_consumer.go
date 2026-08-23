package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/dependency"
	"github.com/andreis3/isura-ledger-ms/internal/infra/factory"
	"github.com/andreis3/isura-ledger-ms/internal/transport/queue"
	"github.com/andreis3/isura-ledger-ms/internal/util"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	DefaultRetryDelay = 5 * time.Second
	MaxBackoffDelay   = 30 * time.Second
	BackoffBase       = 2.0 // Base para o cálculo do backoff exponencial
)

type EventEnvelope struct {
	Type event.Type `json:"type"`
}

type ConsumerMetrics struct {
	processedCount atomic.Int64
	errorCount     atomic.Int64
	processingTime atomic.Int64
}

type NatsConsumerServer struct {
	dep        *dependency.BaseDeps
	publisher  event.Publisher // Publisher injetado para uso em DLQ ou reenvio
	maxWorkers int
	semaphore  chan struct{}
	metrics    *ConsumerMetrics
}

// NewNatsConsumerServer atualizado para receber o event.Publisher por parâmetro
func NewNatsConsumerServer(baseDeps *dependency.BaseDeps, publisher event.Publisher) *NatsConsumerServer {
	return &NatsConsumerServer{
		dep:        baseDeps,
		publisher:  publisher,
		maxWorkers: baseDeps.Cfg.Nats.Consumer.MaxWorkers,
		semaphore:  make(chan struct{}, baseDeps.Cfg.Nats.Consumer.MaxWorkers),
		metrics:    &ConsumerMetrics{},
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

				// Extração de tracing preservando o contexto principal para graceful shutdown
				msgCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Headers()))

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

	<-ctx.Done()
	cc.Stop()

	// Registo das métricas finais
	c.dep.Log.InfoText("Consumer metrics",
		slog.Int64("processed", c.metrics.processedCount.Load()),
		slog.Int64("errors", c.metrics.errorCount.Load()),
		slog.Int64("avg_processing_ms", c.metrics.processingTime.Load()/max(c.metrics.processedCount.Load(), 1)),
	)

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
			AckWait:       cfg.Nats.Consumer.AskWaitSeconds * time.Second,
			MaxDeliver:    cfg.Nats.Consumer.MaxDelivery,
		})
}

func (c *NatsConsumerServer) processJob(ctx context.Context, msg jetstream.Msg) {
	startTime := time.Now()
	defer func() {
		c.metrics.processingTime.Add(time.Since(startTime).Milliseconds())
	}()

	workerCtx, span := c.dep.Tracer.Start(ctx, "NatsConsumerServer.ProcessWorker")
	defer span.End()

	err := c.dispatch(workerCtx, msg)
	if err != nil {
		c.metrics.errorCount.Add(1)
		c.dep.Log.ErrorJSON("Failed to process event",
			"error", err.Error(),
			"subject", msg.Subject(),
		)

		metadata, metaErr := msg.Metadata()

		// 1. Verifica se o erro é permanente ou se excedeu o limite máximo de entregas
		maxDeliveries := uint64(c.dep.Cfg.Nats.Consumer.MaxDelivery)
		if _, ok := errors.AsType[*queue.PermanentError](err); ok || (metaErr == nil && metadata.NumDelivered >= maxDeliveries) {
			_ = msg.Term()
			return
		}

		// 2. Determina o delay com base na política do erro temporário ou cálculo exponencial
		delay := DefaultRetryDelay

		if transErr, ok := errors.AsType[*queue.TransientError](err); ok {
			delay = transErr.Delay

			// Se o handler solicitou explicitamente o envio para DLQ através do TransientError
			if transErr.SendToDLQ {
				c.dep.Log.ErrorJSON("Message marked for DLQ, publishing to dead letter subject",
					"subject", msg.Subject(),
					"error", err.Error(),
				)

				// 1. Cria o evento de Dead Letter encapsulando o subject original e os dados
				dlqEvent := event.NewDeadLetterEvent(msg.Subject(), msg.Data())

				// 2. Publica o evento no broker antes de descartar a mensagem atual
				if pubErr := c.publisher.Publish(ctx, dlqEvent); pubErr != nil {
					c.dep.Log.ErrorJSON("Failed to publish message to Dead Letter Queue", "error", pubErr.Error())
					// Mesmo que a publicação na DLQ falhe, ainda queremos terminar a mensagem
					// para evitar um ciclo infinito, mas registamos o erro crítico.
				}

				// 3. Termina a mensagem original no JetStream
				_ = msg.Term()
				return
			}
		} else if metaErr == nil {
			// Backoff exponencial limpo utilizando a constante BackoffBase
			delay = time.Duration(math.Pow(BackoffBase, float64(metadata.NumDelivered))) * time.Second
			if delay > MaxBackoffDelay {
				delay = MaxBackoffDelay
			}
		}

		_ = msg.NakWithDelay(delay)
		return
	}

	c.metrics.processedCount.Add(1)
	_ = msg.Ack()
}

func (c *NatsConsumerServer) dispatch(ctx context.Context, msg jetstream.Msg) error {
	var envelope EventEnvelope
	if err := util.JsonEngine.Unmarshal(msg.Data(), &envelope); err != nil {
		return fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	switch envelope.Type {
	case event.CreatedBalance:
		h := factory.NewCreateBalanceFactory(c.dep)
		return h.Handle(ctx, msg)

	default:
		return fmt.Errorf("unsupported event type: %s", envelope.Type)
	}
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
