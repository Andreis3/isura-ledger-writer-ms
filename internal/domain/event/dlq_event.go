package event

// DLQSubjectSuffix is ​​the suffix applied to the original subject to form the
// dead-letter subject. The infrastructure uses this constant to register the
// corresponding stream, ensuring that publication and routing do not fall out of sync.
const DLQSubjectSuffix = ".dlq"

// DeadLetterEvent representa um evento que falhou e deve ser enviado para a Dead Letter Queue.
type DeadLetterEvent struct {
	subject string
	payload []byte
}

// Ensures at compile time that DeadLetterEvent implements event.Event
var _ Event = (*DeadLetterEvent)(nil)

// NewDeadLetterEvent creates a new DLQ event targeted at a specific subject (e.g., original.subject.dlq)
func NewDeadLetterEvent(originalSubject string, payload []byte) *DeadLetterEvent {
	return &DeadLetterEvent{
		subject: originalSubject + DLQSubjectSuffix,
		payload: payload,
	}
}

// SubjectName returns the name of the subject where the DLQ event will be published
func (e *DeadLetterEvent) SubjectName() string {
	return e.subject
}

// Payload returns the raw data of the failed message
func (e *DeadLetterEvent) Payload() ([]byte, error) {
	return e.payload, nil
}
