package data

import "context"

// RepoFactory provides access to all repositories and transactional scoping.
// This is the key abstraction that lets the core service work transactionally
// without knowing about pgx.
type RepoFactory interface {
	AssetRepo() AssetRepository
	SourceRepo() SourceRepository
	DocumentRepo() DocumentRepository
	InTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// repoFactoryCtxKey is the context key type for a tx-bound RepoFactory.
type repoFactoryCtxKey struct{}

// WithRepoFactory stores rf in ctx for retrieval inside InTransaction callbacks.
func WithRepoFactory(ctx context.Context, rf RepoFactory) context.Context {
	return context.WithValue(ctx, repoFactoryCtxKey{}, rf)
}

// RepoFactoryFromContext returns the RepoFactory stored by WithRepoFactory,
// plus whether one was found.
func RepoFactoryFromContext(ctx context.Context) (RepoFactory, bool) {
	f, ok := ctx.Value(repoFactoryCtxKey{}).(RepoFactory)
	return f, ok
}
