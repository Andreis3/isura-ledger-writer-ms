package observability

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
)

type ObservabilityHoldRepo struct {
	repo    hold.Repository
	metrics application.Metrics
	tracer  application.Tracer
}

func NewObservabilityHoldRepo(repo hold.Repository, metrics application.Metrics, tracer application.Tracer) *ObservabilityHoldRepo {
	return &ObservabilityHoldRepo{repo: repo, metrics: metrics, tracer: tracer}
}

var _ hold.Repository = (*ObservabilityHoldRepo)(nil)

func (r *ObservabilityHoldRepo) Save(ctx context.Context, instruction *hold.Instruction) error {
	return r.observe(ctx, "Save", "holds", func(ctx context.Context) error { return r.repo.Save(ctx, instruction) })
}
func (r *ObservabilityHoldRepo) Update(ctx context.Context, instruction *hold.Instruction) error {
	return r.observe(ctx, "Update", "holds", func(ctx context.Context) error { return r.repo.Update(ctx, instruction) })
}
func (r *ObservabilityHoldRepo) FindByIdentity(ctx context.Context, origin, reference string) (*hold.Instruction, error) {
	var value *hold.Instruction
	err := r.observe(ctx, "FindByIdentity", "holds", func(ctx context.Context) error {
		var err error
		value, err = r.repo.FindByIdentity(ctx, origin, reference)
		return err
	})
	return value, err
}
func (r *ObservabilityHoldRepo) FindByID(ctx context.Context, id string) (*hold.Instruction, error) {
	var value *hold.Instruction
	err := r.observe(ctx, "FindByID", "holds", func(ctx context.Context) error { var err error; value, err = r.repo.FindByID(ctx, id); return err })
	return value, err
}
func (r *ObservabilityHoldRepo) FindEffectiveByAccount(ctx context.Context, accountID string, currency money.Currency) ([]hold.Instruction, error) {
	var values []hold.Instruction
	err := r.observe(ctx, "FindEffectiveByAccount", "holds", func(ctx context.Context) error {
		var err error
		values, err = r.repo.FindEffectiveByAccount(ctx, accountID, currency)
		return err
	})
	return values, err
}
func (r *ObservabilityHoldRepo) SaveHistory(ctx context.Context, history hold.History) error {
	return r.observe(ctx, "SaveHistory", "hold_history", func(ctx context.Context) error { return r.repo.SaveHistory(ctx, history) })
}
func (r *ObservabilityHoldRepo) FindHistory(ctx context.Context, holdID string) ([]hold.History, error) {
	var values []hold.History
	err := r.observe(ctx, "FindHistory", "hold_history", func(ctx context.Context) error {
		var err error
		values, err = r.repo.FindHistory(ctx, holdID)
		return err
	})
	return values, err
}
func (r *ObservabilityHoldRepo) FindRelease(ctx context.Context, key string) (*hold.Release, error) {
	var value *hold.Release
	err := r.observe(ctx, "FindRelease", "hold_releases", func(ctx context.Context) error { var err error; value, err = r.repo.FindRelease(ctx, key); return err })
	return value, err
}
func (r *ObservabilityHoldRepo) SaveRelease(ctx context.Context, release hold.Release) error {
	return r.observe(ctx, "SaveRelease", "hold_releases", func(ctx context.Context) error { return r.repo.SaveRelease(ctx, release) })
}

func (r *ObservabilityHoldRepo) observe(ctx context.Context, method, table string, operation func(context.Context) error) error {
	ctx, span := r.tracer.Start(ctx, "HoldRepository."+method)
	defer span.End()
	started := time.Now()
	defer func() {
		r.metrics.RecordDBQueryDuration("postgres", table, method, float64(time.Since(started).Milliseconds()))
	}()
	if err := operation(ctx); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
