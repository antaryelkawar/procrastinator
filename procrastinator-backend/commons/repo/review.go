package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// HoldInput carries the data the review service needs to hold a low-confidence
// extraction for human review.
type HoldInput struct {
	// Extraction is the parsed LLM extraction for the uploaded document.
	Extraction entity.Extraction
	// SourceID is the retained source's ID (the upload that produced this extraction).
	SourceID string
	// OwnerHouseholdID fences the identity match and scopes the review row
	// to a household; nil means personal (no household).
	OwnerHouseholdID *string
}

// Reviewer holds a low-confidence extraction for human review, creating a
// pending IngestReview row linked to the best-matched asset.
type Reviewer interface {
	// Hold creates a pending IngestReview for the given extraction and source.
	// It resolves the best-matched asset via identity matching and records
	// the result as a snapshot on the review row.
	Hold(ctx context.Context, input HoldInput) (entity.IngestReview, error)
}
