package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

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
			toMap:     reviewToMap,
			filters:   reviewFilters,
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
			toMap:     reviewToMap,
			filters:   reviewFilters,
			shareable: true,
		},
	}
}

// scanReview scans a row into an entity.IngestReview, mapping pgx.ErrNoRows to
// repo.ErrNotFound. Column order matches the ingest_reviews table (migration
// 00005 + 00006). provenance was added in 00006.
func scanReview(row rowScanner) (entity.IngestReview, error) {
	var r entity.IngestReview
	var id string
	var candidateFieldsJSON, rawJSON, provenanceJSON []byte
	err := row.Scan(
		&id, &r.OwnerID, &r.OwnerHouseholdID, &r.SourceID, &r.DocType,
		&candidateFieldsJSON, &rawJSON, &r.Confidence, &r.BestMatchedAssetID,
		&r.State, &r.CreatedAt, &r.DecidedAt, &r.DecidedBy,
		&provenanceJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.IngestReview{}, repo.ErrNotFound
		}
		return entity.IngestReview{}, err
	}
	r.ID = id
	if len(candidateFieldsJSON) > 0 {
		if err := json.Unmarshal(candidateFieldsJSON, &r.CandidateFields); err != nil {
			return entity.IngestReview{}, fmt.Errorf("decode candidate_fields: %w", err)
		}
	}
	if len(rawJSON) > 0 {
		if err := json.Unmarshal(rawJSON, &r.RawExtraction); err != nil {
			return entity.IngestReview{}, fmt.Errorf("decode raw_extraction: %w", err)
		}
	}
	if len(provenanceJSON) > 0 {
		if err := json.Unmarshal(provenanceJSON, &r.Provenance); err != nil {
			return entity.IngestReview{}, fmt.Errorf("decode provenance: %w", err)
		}
	}
	return r, nil
}

// reviewToMap maps an entity.IngestReview to its SQL column→value pairs,
// including only non-zero/non-nil fields.
func reviewToMap(r entity.IngestReview) map[string]any {
	m := make(map[string]any)
	if r.ID != "" {
		m["id"] = r.ID
	}
	if r.OwnerID != "" {
		m["owner_id"] = r.OwnerID
	}
	if r.OwnerHouseholdID != nil {
		m["owner_household_id"] = r.OwnerHouseholdID
	}
	if r.SourceID != "" {
		m["source_id"] = r.SourceID
	}
	if r.DocType != "" {
		m["doc_type"] = r.DocType
	}
	if r.CandidateFields != nil {
		fields, err := toMetadataJSON(r.CandidateFields)
		if err == nil {
			m["candidate_fields"] = fields
		}
	}
	if r.RawExtraction != "" {
		raw, err := json.Marshal(r.RawExtraction)
		if err == nil {
			m["raw_extraction"] = raw
		}
	}
	if r.Confidence != nil {
		m["confidence"] = r.Confidence
	}
	if r.BestMatchedAssetID != nil {
		m["best_matched_asset_id"] = r.BestMatchedAssetID
	}
	if r.State != "" {
		m["state"] = string(r.State)
	}
	if r.DecidedAt != nil {
		m["decided_at"] = r.DecidedAt
	}
	if r.DecidedBy != nil {
		m["decided_by"] = r.DecidedBy
	}
	if r.Provenance != nil {
		prov, err := toMetadataJSON(r.Provenance)
		if err == nil {
			m["provenance"] = prov
		}
	}
	return m
}

var reviewFieldCols = map[string]string{
	"id":                    "id",
	"state":                 "state",
	"created_at":            "created_at",
	"source_id":             "source_id",
	"doc_type":              "doc_type",
	"owner_household_id":    "owner_household_id",
	"best_matched_asset_id": "best_matched_asset_id",
	"decided_at":            "decided_at",
}

var reviewOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"state":      "state",
}

var reviewFilters = filterConfig{fieldCols: reviewFieldCols, orderCols: reviewOrderCols}
