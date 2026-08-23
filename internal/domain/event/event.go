package event

import "context"

type Type string

const (
	CreatedBalance Type = "created_balance"
)

// Event define o contrato que todo evento de domínio precisa cumprir
type Event interface {
	SubjectName() string
	Payload() ([]byte, error)
}

// Publisher define o contrato para disparar eventos para o broker
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}
