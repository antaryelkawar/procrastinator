package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/repo"
)

// NewFactory returns a repo.Factory whose repositories are bound to pool.
// The InTx callback provides transactional scoping with tx-bound repositories.
func NewFactory(pool *pgxpool.Pool) *repo.Factory {
	return &repo.Factory{
		Assets:    NewAssetRepository(pool),
		Sources:   NewSourceRepository(pool),
		Documents: NewDocumentRepository(pool),
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("postgres: begin tx: %w", err)
			}
			defer tx.Rollback(ctx) // no-op if committed

			repos := &repo.Repos{
				Assets:    NewAssetRepository(tx),
				Sources:   NewSourceRepository(tx),
				Documents: NewDocumentRepository(tx),
			}

			if err := fn(ctx, repos); err != nil {
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("postgres: commit tx: %w", err)
			}
			return nil
		},
	}
}
