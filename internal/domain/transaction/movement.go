package transaction

import (
	"errors"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

// Movement groups entries that describe one balanced part of a transaction.
// Its value is derived from Entries and is not stored separately.
type Movement struct {
	ID          entity.ID
	Type        string
	Description string
	Entries     []*Entry
}

type MovementBuilder struct {
	id          entity.ID
	typeName    string
	description string
	entries     []*Entry
	eval        validator.Evaluator
}

func NewMovementBuilder() *MovementBuilder { return &MovementBuilder{} }

func (b *MovementBuilder) WithID(id ...string) *MovementBuilder {
	if len(id) > 0 {
		b.eval.CheckField(validator.NotBlank(id[0]), "id", "cannot be blank")
		b.eval.CheckField(validator.MatchesUUIDv7(id[0]), "id", "is not uuid")
		parsed, err := entity.NewID(id[0])
		if err != nil {
			b.eval.AddFieldError("id", err.Error())
		}
		b.id = parsed
		return b
	}
	generatedID, err := entity.NewIDV7()
	if err != nil {
		b.eval.AddFieldError("id", err.Error())
	}
	b.id = generatedID
	return b
}

func (b *MovementBuilder) WithType(movementType string) *MovementBuilder {
	b.eval.CheckField(strings.TrimSpace(movementType) != "", "type", "cannot be blank")
	b.typeName = movementType
	return b
}

func (b *MovementBuilder) WithDescription(description string) *MovementBuilder {
	b.eval.CheckField(strings.TrimSpace(description) != "", "description", "cannot be blank")
	b.description = description
	return b
}

func (b *MovementBuilder) WithEntries(entries []*Entry) *MovementBuilder {
	b.entries = append([]*Entry(nil), entries...)
	b.eval.CheckField(len(b.entries) > 0, "entries", "must contain at least one entry")
	for _, entry := range b.entries {
		b.eval.CheckField(entry != nil, "entries", "entry cannot be nil")
	}
	return b
}

func (b *MovementBuilder) Build() (*Movement, error) {
	if len(b.eval) > 0 {
		return nil, fault.InvalidEntityError(errors.New("invalid movement entity"), b.eval)
	}
	return &Movement{ID: b.id, Type: b.typeName, Description: b.description, Entries: append([]*Entry(nil), b.entries...)}, nil
}
