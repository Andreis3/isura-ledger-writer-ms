package nats

import (
	"context"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Publisher struct {
	js     jetstream.JetStream
	tracer application.Tracer
}

var _ event.Publisher = (*Publisher)(nil)

func NewJetStreamPublisher(js jetstream.JetStream, tracer application.Tracer) *Publisher {
	return &Publisher{
		js:     js,
		tracer: tracer,
	}
}

func (p *Publisher) Publish(ctx context.Context, event event.Event) error {
	ctx, span := p.tracer.Start(ctx, "Publisher.Publish")
	defer span.End()
	payload, err := event.Payload()
	if err != nil {
		return err
	}

	// Cria a mensagem NATS suportando headers
	msg := &nats.Msg{
		Subject: event.SubjectName(),
		Data:    payload,
		Header:  make(nats.Header),
	}

	// Injeta o trace_id e spans atuais (do HTTP) nos headers do NATS
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))

	// Publica a mensagem com os headers embutidos
	_, err = p.js.PublishMsg(ctx, msg)
	if err != nil {
		return err
	}

	return nil
}
