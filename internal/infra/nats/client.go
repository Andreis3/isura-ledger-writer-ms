package nats

import (
	"context"
	"fmt"
	"time"

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

func SetupStreams(ctx context.Context, js jetstream.JetStream, cfg *configs.Configs) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.Nats.Name,              // Standardized name matching the Consumer
		Subjects:  []string{cfg.Nats.Subject}, // Captures any event matching the configured subject
		Storage:   jetstream.FileStorage,      // Persists on disk
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Replicas:  1, // 1 replica for local/Docker environment (avoids cluster error)
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return fmt.Errorf("failed to create or update stream: %w", err)
	}
	return nil
}

func (n *ClientNats) Close() {
	n.JS.Conn().Close()
}
