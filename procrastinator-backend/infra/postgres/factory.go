package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// searchBackend implements repo.SearchBackend by forwarding each search call to
// the corresponding pool-bound repository.
type searchBackend struct {
	assets        *AssetRepository
	accounts      *AccountRepository
	movements     *MovementRepository
	documents     *DocumentRepository
	importBatches *ImportBatchRepository
}

// Compile-time guard: the adapter satisfies the search backend interface.
var _ repo.SearchBackend = (*searchBackend)(nil)

func (s *searchBackend) SearchAssets(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.Asset, error) {
	return s.assets.SearchAssets(ctx, pattern, opts...)
}

func (s *searchBackend) SearchAccounts(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.FinancialAccount, error) {
	return s.accounts.SearchAccounts(ctx, pattern, opts...)
}

func (s *searchBackend) SearchMovements(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	return s.movements.SearchMovements(ctx, pattern, opts...)
}

func (s *searchBackend) SearchDocuments(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.Document, error) {
	return s.documents.SearchDocuments(ctx, pattern, opts...)
}

func (s *searchBackend) SearchImportBatches(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.ImportBatch, error) {
	return s.importBatches.SearchImportBatches(ctx, pattern, opts...)
}

// NewFactory returns a repo.Factory whose repositories are bound to pool.
// The InTx callback provides transactional scoping with tx-bound repositories.
func NewFactory(pool *pgxpool.Pool) *repo.Factory {
	assets := NewAssetRepository(pool)
	sources := NewSourceRepository(pool)
	documents := NewDocumentRepository(pool)
	accounts := NewAccountRepository(pool)
	movements := NewMovementRepository(pool)
	importBatches := NewImportBatchRepository(pool)
	importLines := NewImportLineRepository(pool)

	return &repo.Factory{
		Assets:        assets,
		Sources:       sources,
		Documents:     documents,
		Accounts:      accounts,
		Movements:     movements,
		ImportBatches: importBatches,
		ImportLines:   importLines,
		Households:    NewHouseholdRepository(pool),
		Users:         NewUserRegistry(pool),
		Search: &searchBackend{
			assets:        assets,
			accounts:      accounts,
			movements:     movements,
			documents:     documents,
			importBatches: importBatches,
		},
		Reviews: NewReviewRepository(pool),
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("postgres: begin tx: %w", err)
			}
			defer tx.Rollback(ctx) // no-op if committed

			repos := &repo.Repos{
				Assets:        newAssetRepoForTx(tx),
				Sources:       newSourceRepoForTx(tx),
				Documents:     newDocumentRepoForTx(tx),
				Accounts:      newAccountRepoForTx(tx),
				Movements:     newMovementRepoForTx(tx),
				ImportBatches: newImportBatchRepoForTx(tx),
				ImportLines:   newImportLineRepoForTx(tx),
				Households:    newHouseholdRepoForTx(tx),
				Reviews:       newReviewRepoForTx(tx),
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
