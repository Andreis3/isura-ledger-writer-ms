package application

import "context"

type RepositoryFactory func(tx any) any

type UnitOfWork interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	WithRetryableTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
