package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/domain/event"
	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type ClientNats struct {
	JS  jetstream.JetStream
	cfg *configs.Configs
}

func NewJetStreamConnection(cfg *configs.Configs) (*ClientNats, error) {
	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	ctx := context.Background()

	if err := SetupStreams(ctx, js, cfg); err != nil {
		return nil, err
	}

	return &ClientNats{JS: js}, nil
}

const (
	// DLQStreamSuffix keeps dead letters in their own stream so the main stream's
	// retention policy cannot discard messages that still need manual inspection.
	DLQStreamSuffix = "_DLQ"

	streamMaxAge = 7 * 24 * time.Hour
	dlqMaxAge    = 30 * 24 * time.Hour
)

func SetupStreams(ctx context.Context, js jetstream.JetStream, cfg *configs.Configs) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.Nats.Name,              // Standardized name matching the Consumer
		Subjects:  []string{cfg.Nats.Subject}, // Captures any event matching the configured subject
		Storage:   jetstream.FileStorage,      // Persists on disk
		Retention: jetstream.LimitsPolicy,
		MaxAge:    streamMaxAge,
		Replicas:  1, // 1 replica for local/Docker environment (avoids cluster error)
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return fmt.Errorf("failed to create or update stream: %w", err)
	}

	// Dead letters are published to <subject>.dlq, which the main stream does not
	// capture. Without this stream the publish fails with "no stream response" and
	// the failed message is lost instead of being parked for inspection.
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.Nats.Name + DLQStreamSuffix,
		Subjects:  []string{cfg.Nats.Subject + event.DLQSubjectSuffix},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    dlqMaxAge,
		Replicas:  1,
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return fmt.Errorf("failed to create or update dead letter stream: %w", err)
	}

	return nil
}

func (n *ClientNats) Close() {
	n.JS.Conn().Close()
}
