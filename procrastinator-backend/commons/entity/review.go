package entity

import "time"

// IngestReviewState represents the lifecycle state of an ingest review.
// Values must match the ingest_reviews.state CHECK constraint in migration 00005.
type IngestReviewState string

const (
	// ReviewStatePending is the initial state: the upload was held for human review.
	ReviewStatePending IngestReviewState = "pending"
	// ReviewStateApproved marks a review that was approved (auto-committed path taken).
	ReviewStateApproved IngestReviewState = "approved"
	// ReviewStateRejected marks a review that was rejected (upload discarded).
	ReviewStateRejected IngestReviewState = "rejected"
)

// IngestReview represents a held (pending-review) upload that was not
// auto-committed due to low or absent extraction confidence. One row in the
// ingest_reviews table (migration 00005).
type IngestReview struct {
	ID              string
	OwnerID         string
	SourceID        string
	DocType         string
	CandidateFields map[string]any
	RawExtraction   string
	State           IngestReviewState
	CreatedAt       time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
	// Confidence is the LLM extraction confidence (0.0–1.0); nil means not yet scored.
	Confidence *float64
	// BestMatchedAssetID is a snapshot of the best-matching asset at extraction time;
	// nil means no candidate asset was found (FK ON DELETE SET NULL).
	BestMatchedAssetID *string
	// DecidedAt is when the review was approved/rejected; nil while pending.
	DecidedAt *time.Time
	// DecidedBy is the user who decided the review; nil while pending.
	DecidedBy *string
	// Provenance stores the per-worker extraction results and/or candidate set
	// that produced this review row. Maps to ingest_reviews.provenance (jsonb).
	Provenance map[string]any
	UpdatedAt  time.Time
	// DeletedAt is the soft-delete timestamp; nil means the review is active.
	DeletedAt *time.Time
}

// GetID returns the entity's row identity.
func (r IngestReview) GetID() string { return r.ID }
