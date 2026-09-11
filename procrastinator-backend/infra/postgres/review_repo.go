package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// ReviewRepository is the generic repository engine for entity.IngestReview.
type ReviewRepository struct {
	*pgRepository[entity.IngestReview]
}

// Compile-time guard: the concrete review repository satisfies the generic
// repository interface for its entity.
var _ repo.Repository[entity.IngestReview] = (*ReviewRepository)(nil)

// NewReviewRepository returns a repository for entity.IngestReview.
func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{
		pgRepository: &pgRepository[entity.IngestReview]{
			scope:     &poolScope{pool: pool},
			table:     "ingest_reviews",
			scanRow:   scanReview,
			codec:     reviewCodec,
			shareable: true,
		},
	}
}

// newReviewRepoForTx returns a review repository bound to an ambient transaction.
func newReviewRepoForTx(tx pgx.Tx) *ReviewRepository {
	return &ReviewRepository{
		pgRepository: &pgRepository[entity.IngestReview]{
			scope:     &txScopeImpl{tx: tx},
			table:     "ingest_reviews",
			scanRow:   scanReview,
			codec:     reviewCodec,
			shareable: true,
		},
	}
}
