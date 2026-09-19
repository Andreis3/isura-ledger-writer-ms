package observability

import (
	"context"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

type ObservabilityTransactionRepo struct {
	repo   transaction.Repository
	metric application.Metrics
	tracer application.Tracer
}

func NewObservabilityTransactionRepo(repo transaction.Repository, metric application.Metrics, tracer application.Tracer) *ObservabilityTransactionRepo {
	return &ObservabilityTransactionRepo{
		repo:   repo,
		metric: metric,
		tracer: tracer,
	}
}

func (r *ObservabilityTransactionRepo) Save(ctx context.Context, data *transaction.Transaction) error {
	ctx, span := r.tracer.Start(ctx, "TransactionRepository.Save")
	defer span.End()

	start := time.Now()
	defer func() {
		r.metric.RecordDBQueryDuration(
			"postgres",
			"transactions",
			"save",
			float64(time.Since(start).Milliseconds()))
	}()

	err := r.repo.Save(ctx, data)

	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

func (r *ObservabilityTransactionRepo) Find(ctx context.Context, params criteria.TransactionCriteria) (*transaction.Transaction, error) {
	ctx, span := r.tracer.Start(ctx, "TransactionRepository.Find")
	defer span.End()

	start := time.Now()
	defer func() {
		r.metric.RecordDBQueryDuration(
			"postgres",
			"transactions",
			"find",
			float64(time.Since(start).Milliseconds()))
	}()

	transactionResponse, err := r.repo.Find(ctx, params)

	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return transactionResponse, nil
}

func (r *ObservabilityTransactionRepo) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	ctx, span := r.tracer.Start(ctx, "TransactionRepository.ExistsByIdempotencyKey")
	defer span.End()

	start := time.Now()
	defer func() {
		r.metric.RecordDBQueryDuration(
			"postgres",
			"transactions",
			"exists_by_idempotency_key",
			float64(time.Since(start).Milliseconds()))
	}()

	exists, err := r.repo.ExistsByIdempotencyKey(ctx, idempotencyKey)

	if err != nil {
		span.RecordError(err)
		return false, err
	}

	return exists, nil
}
