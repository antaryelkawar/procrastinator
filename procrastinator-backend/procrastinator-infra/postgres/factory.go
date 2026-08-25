package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	data "procrastinator-backend/commons-data"
)

var (
	_ data.RepoFactory = (*pgRepoFactory)(nil)
	_ data.RepoFactory = (*txRepoFactory)(nil)
)

// pgRepoFactory implements data.RepoFactory over a pgx connection pool.
type pgRepoFactory struct {
	pool *pgxpool.Pool
}

// NewRepoFactory returns a RepoFactory whose repositories are bound to pool.
func NewRepoFactory(pool *pgxpool.Pool) *pgRepoFactory {
	return &pgRepoFactory{pool: pool}
}

// AssetRepo returns an asset repository bound to the pool.
func (f *pgRepoFactory) AssetRepo() data.AssetRepository {
	return NewAssetRepo(f.pool)
}

// SourceRepo returns a source repository bound to the pool.
func (f *pgRepoFactory) SourceRepo() data.SourceRepository {
	return NewSourceRepo(f.pool)
}

// DocumentRepo returns a document repository bound to the pool.
func (f *pgRepoFactory) DocumentRepo() data.DocumentRepository {
	return NewDocumentRepo(f.pool)
}

// InTransaction runs fn inside a single database transaction. The context
// passed to fn carries a tx-bound factory (see RepoFactoryFromContext) whose
// repositories all share the transaction. If fn returns nil the transaction
// is committed; otherwise it is rolled back and fn's error is returned.
func (f *pgRepoFactory) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op if already committed

	ctx = data.WithRepoFactory(ctx, &txRepoFactory{tx: tx})

	if err := fn(ctx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit tx: %w", err)
	}
	return nil
}

// txRepoFactory is a RepoFactory whose repositories share one transaction.
// Nested transactions are not supported: InTransaction runs fn as-is.
type txRepoFactory struct {
	tx pgx.Tx
}

// AssetRepo returns an asset repository bound to the transaction.
func (f *txRepoFactory) AssetRepo() data.AssetRepository {
	return NewAssetRepo(f.tx)
}

// SourceRepo returns a source repository bound to the transaction.
func (f *txRepoFactory) SourceRepo() data.SourceRepository {
	return NewSourceRepo(f.tx)
}

// DocumentRepo returns a document repository bound to the transaction.
func (f *txRepoFactory) DocumentRepo() data.DocumentRepository {
	return NewDocumentRepo(f.tx)
}

// InTransaction does not support nested transactions: it simply runs fn with
// the given ctx and returns its error. The surrounding transaction is left to
// the outer InTransaction to commit or roll back.
func (f *txRepoFactory) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

