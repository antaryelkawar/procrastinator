package processing

import (
	"procrastinator-backend/commons/entity"
)

// OutcomeKind classifies the result of processing a single upload.
type OutcomeKind string

// Processing outcome kinds (design D1).
const (
	// OutcomeCommitted means the document resolved to an asset that was
	// created or merged and a document was persisted.
	OutcomeCommitted OutcomeKind = "committed"
	// OutcomeHeldForReview means the document could not be auto-committed
	// (below the confidence threshold or an ambiguous match) and was held
	// for human review.
	OutcomeHeldForReview OutcomeKind = "held_for_review"
	// OutcomeDuplicate means the upload is a byte-identical re-upload of an
	// existing source for the owner; no new source, document, or asset was
	// created.
	OutcomeDuplicate OutcomeKind = "duplicate"
	// OutcomeFailed means hard processing failure (all workers failed,
	// oversize, unsupported type, ...). Reason carries the detail.
	OutcomeFailed OutcomeKind = "failed"
	// OutcomeStatement means the document was classified as a statement and
	// should be routed to the ledger import pipeline. Statement carries the
	// retained source.
	OutcomeStatement OutcomeKind = "statement"
)

// Duplicate references the existing source/document/asset that a re-upload
// maps to. AssetDeleted is true when the linked asset is soft-deleted, in
// which case the UI should offer a restore instead of reprocessing.
type Duplicate struct {
	SourceID     string
	DocumentID   string
	AssetID      string
	AssetDeleted bool
}

// Provenance carries per-worker extraction results, errors, and the candidate
// set that produced an outcome. It persists to ingest_reviews.provenance
// (jsonb) on the hold path and travels with every outcome for audit.
type Provenance map[string]any

// Outcome is the discriminated result of processing one upload. Only the
// fields relevant to Kind are populated: Committed→Asset, HeldForReview→Review,
// Duplicate→Duplicate, Statement→Statement, Failed→Reason.
type Outcome struct {
	Kind OutcomeKind
	// Asset is the created/merged asset (Kind == OutcomeCommitted).
	Asset *entity.Asset
	// Review is the held-for-review row (Kind == OutcomeHeldForReview).
	Review *entity.IngestReview
	// Duplicate references the existing source/document/asset
	// (Kind == OutcomeDuplicate).
	Duplicate *Duplicate
	// Statement is the retained source to route to the ledger
	// (Kind == OutcomeStatement).
	Statement *entity.Source
	// Reason carries the failure detail (Kind == OutcomeFailed).
	Reason string
	// Confidence is the consensus confidence, populated for the Committed and
	// HeldForReview outcomes; nil when absent.
	Confidence *float64
	// Provenance carries per-worker results + the candidate set for audit.
	Provenance Provenance
}
