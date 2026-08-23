package event

// DeadLetterEvent representa um evento que falhou e deve ser enviado para a Dead Letter Queue.
type DeadLetterEvent struct {
	subject string
	payload []byte
}

// Garante em tempo de compilação que DeadLetterEvent implementa event.Event
var _ Event = (*DeadLetterEvent)(nil)

// NewDeadLetterEvent cria um novo evento de DLQ direcionado para um subject específico (ex: original.subject.dlq)
func NewDeadLetterEvent(originalSubject string, payload []byte) *DeadLetterEvent {
	return &DeadLetterEvent{
		subject: originalSubject + ".dlq", // Sufixo comum para tópicos de DLQ
		payload: payload,
	}
}

// SubjectName retorna o nome do subject onde o evento de DLQ será publicado
func (e *DeadLetterEvent) SubjectName() string {
	return e.subject
}

// Payload retorna os dados brutos da mensagem que falhou
func (e *DeadLetterEvent) Payload() ([]byte, error) {
	return e.payload, nil
}
