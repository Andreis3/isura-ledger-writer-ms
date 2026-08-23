package types

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// Interface que define o contrato dos Handlers de fila
type QueueHandler interface {
	Handle(ctx context.Context, msg jetstream.Msg) error
}
