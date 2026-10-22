package query

import (
	"context"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

type GetHold struct{ repository hold.Repository }

func NewGetHold(repository hold.Repository) *GetHold { return &GetHold{repository: repository} }

func (q *GetHold) ByID(ctx context.Context, id string) (*hold.Instruction, []hold.History, error) {
	instruction, err := q.repository.FindByID(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("find hold: %w", err)
	}
	if instruction == nil {
		return nil, nil, nil
	}
	history, err := q.repository.FindHistory(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("find hold history: %w", err)
	}
	return instruction, history, nil
}

func (q *GetHold) EffectiveByAccount(ctx context.Context, accountID string, currency money.Currency) ([]hold.Instruction, error) {
	result, err := q.repository.FindEffectiveByAccount(ctx, accountID, currency)
	if err != nil {
		return nil, fmt.Errorf("find effective holds: %w", err)
	}
	return result, nil
}
