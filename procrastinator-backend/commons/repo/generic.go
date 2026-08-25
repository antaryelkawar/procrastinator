package repo

import "context"

// Repository provides generic CRUD operations for entity type T.
// Options configure tenant scoping, filtering, pagination, and ordering.
type Repository[T any] interface {
	Get(ctx context.Context, id string, opts ...Option) (T, error)
	List(ctx context.Context, opts ...Option) ([]T, error)
	Create(ctx context.Context, entity T, opts ...Option) (T, error)
	Update(ctx context.Context, entity T, opts ...Option) (T, error)
	Delete(ctx context.Context, id string, opts ...Option) error
}
