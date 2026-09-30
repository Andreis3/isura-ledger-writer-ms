package adapter

import (
	"context"

	relayNATS "github.com/andreis3/isura-ledger-ms/internal/infra/nats"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"
)

type JetStreamMock struct{ mock.Mock }

var _ relayNATS.JetStreamPublisher = (*JetStreamMock)(nil)

func (m *JetStreamMock) PublishMsg(ctx context.Context, msg *nats.Msg, options ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	args := m.Called(ctx, msg, options)
	var ack *jetstream.PubAck
	if value := args.Get(0); value != nil {
		ack = value.(*jetstream.PubAck)
	}
	return ack, args.Error(1)
}
