package event

import (
	"encoding/json"
)

type CreateBalance struct {
	Type      Type   `json:"type"`
	AccountID string `json:"account_id"`
	Currency  string `json:"currency"`
}

func NewCreateBalanceEvent(accountID, currency string) *CreateBalance {
	return &CreateBalance{
		Type:      CreatedBalance,
		AccountID: accountID,
		Currency:  currency,
	}
}

// SubjectName define o canal/tópico no NATS JetStream
func (e *CreateBalance) SubjectName() string {
	return "ledger.writer.event"
}

// Payload serializa a struct para JSON
func (e *CreateBalance) Payload() ([]byte, error) {
	return json.Marshal(e)
}
