// Package review implements the confidence-review bounded context: it holds
// low-confidence extractions for human review (Hold), then commits the
// candidate asset and document on approval (Approve) or discards the upload
// on rejection (Reject). It is user-scoped: every method resolves the bound
// user from the context first and fails closed (user.ErrNoUser) without
// touching any repository when it is absent.
package review

import (
	"context"
	"errors"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/identity"
)

// ErrConflict is returned when attempting to approve or reject a review
// that is not in the pending state. The API layer maps this sentinel to
// HTTP 409.
var ErrConflict = errors.New("review is not in a pending state")

// Service is the review application service. It is user-scoped: every method
// resolves the user from the context first and fails closed (user.ErrNoUser)
// without touching any repository when it is absent.
type Service struct {
	factory *repo.Factory
}

var _ repo.Reviewer = (*Service)(nil)

// New constructs a Service over the given repository factory.
func New(factory *repo.Factory) *Service {
	return &Service{factory: factory}
}

// Hold creates a pending IngestReview for the given extraction and source.
// It resolves the best-matched asset via identity matching (read-only) and
// records the result as a snapshot on the review row. The match is performed
// inside the same transaction as the insert so the snapshot is consistent
// with the state of the asset table at review-creation time. ErrNoIdentity
// is propagated (the gate that decides whether to Hold already checked, but
// the service defends against a caller bypassing the gate).
func (s *Service) Hold(ctx context.Context, input repo.HoldInput) (entity.IngestReview, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.IngestReview{}, err
	}

	var created entity.IngestReview
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		matched, found, err := identity.Match(ctx, r.Assets, input.Extraction, input.OwnerHouseholdID)
		if err != nil {
			return err
		}
		var bestMatchID *string
		if found {
			bestMatchID = &matched.ID
		}

		created, err = r.Reviews.Create(ctx, entity.IngestReview{
			SourceID:           input.SourceID,
			DocType:            input.Extraction.Classification,
			CandidateFields:    candidateFields(input.Extraction),
			RawExtraction:      input.Extraction.RawPayload,
			State:              entity.ReviewStatePending,
			OwnerHouseholdID:   input.OwnerHouseholdID,
			Confidence:         input.Extraction.Confidence,
			BestMatchedAssetID: bestMatchID,
		}, repo.Owner(tid))
		return err
	})
	if err != nil {
		return entity.IngestReview{}, err
	}
	return created, nil
}

// Approve atomically commits the candidate: it loads the pending review,
// commits the extraction onto the best-matched asset (or creates a new asset
// when no match was found), persists a Document row, and transitions the
// review to approved. All four writes happen in one transaction so an
// approval is all-or-nothing. The state guard (pending-only) prevents
// double-decisions and races between Approve/Reject.
func (s *Service) Approve(ctx context.Context, id string) (entity.Asset, entity.IngestReview, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, entity.IngestReview{}, err
	}

	var asset entity.Asset
	var updated entity.IngestReview
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		rev, err := r.Reviews.Get(ctx, id, repo.Owner(tid))
		if err != nil {
			return err
		}
		if rev.State != entity.ReviewStatePending {
			return ErrConflict
		}

		ext := rehydrateExtraction(rev.CandidateFields, rev.RawExtraction)

		a, err := identity.CommitCandidate(ctx, r.Assets, ext, rev.BestMatchedAssetID, rev.OwnerHouseholdID)
		if err != nil {
			return err
		}
		asset = a

		doc := entity.Document{
			SourceID:         rev.SourceID,
			AssetID:          a.ID,
			DocType:          rev.DocType,
			ExtractedFields:  rev.CandidateFields,
			RawExtraction:    rev.RawExtraction,
			OwnerHouseholdID: rev.OwnerHouseholdID,
			Confidence:       rev.Confidence,
		}
		if _, err = r.Documents.Create(ctx, doc, repo.Owner(tid)); err != nil {
			return err
		}

		now := time.Now().UTC()
		rev.State = entity.ReviewStateApproved
		rev.DecidedAt = &now
		rev.DecidedBy = &tid
		updated, err = r.Reviews.Update(ctx, rev, repo.Owner(tid))
		return err
	})
	if err != nil {
		return entity.Asset{}, entity.IngestReview{}, err
	}
	return asset, updated, nil
}

// Reject transitions a pending review to rejected. It loads the review,
// guards that it is still pending, stamps DecidedAt/DecidedBy, and persists
// the transition. No asset or document is written: the upload is discarded.
func (s *Service) Reject(ctx context.Context, id string) (entity.IngestReview, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.IngestReview{}, err
	}

	var updated entity.IngestReview
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		rev, err := r.Reviews.Get(ctx, id, repo.Owner(tid))
		if err != nil {
			return err
		}
		if rev.State != entity.ReviewStatePending {
			return ErrConflict
		}
		now := time.Now().UTC()
		rev.State = entity.ReviewStateRejected
		rev.DecidedAt = &now
		rev.DecidedBy = &tid
		updated, err = r.Reviews.Update(ctx, rev, repo.Owner(tid))
		return err
	})
	if err != nil {
		return entity.IngestReview{}, err
	}
	return updated, nil
}

// List returns the bound user's reviews, filtered by state. An empty status
// defaults to ReviewStatePending. Results are ordered by created_at, then
// id, so the oldest pending review is first (stable, deterministic paging).
func (s *Service) List(ctx context.Context, status entity.IngestReviewState) ([]entity.IngestReview, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = entity.ReviewStatePending
	}
	opts := []repo.Option{
		repo.Owner(tid),
		repo.Where("state", "=", string(status)),
		repo.OrderBy("created_at, id"),
	}
	return s.factory.Reviews.List(ctx, opts...)
}

// Get returns a single review by id, scoped to the bound user. repo.ErrNotFound
// propagates untouched (the API maps it to HTTP 404).
func (s *Service) Get(ctx context.Context, id string) (entity.IngestReview, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.IngestReview{}, err
	}
	return s.factory.Reviews.Get(ctx, id, repo.Owner(tid))
}

// candidateFields flattens the extraction into a non-nil map containing a
// key only for each present value. Mirrors ingest.extractionFields so the
// stored candidate_fields shape is identical across the auto-commit and
// held paths; Confidence is intentionally not included here (it is a
// separate column on the review row).
func candidateFields(ext entity.Extraction) map[string]any {
	fields := make(map[string]any, 9)
	if ext.Classification != "" {
		fields["classification"] = ext.Classification
	}
	if ext.Brand != nil {
		fields["brand"] = *ext.Brand
	}
	if ext.Model != nil {
		fields["model"] = *ext.Model
	}
	if ext.SerialNumber != nil {
		fields["serial_number"] = *ext.SerialNumber
	}
	if ext.Price != nil {
		fields["price"] = *ext.Price
	}
	if ext.Currency != nil {
		fields["currency"] = *ext.Currency
	}
	if ext.PurchaseDate != nil {
		fields["purchase_date"] = ext.PurchaseDate.Format(time.DateOnly)
	}
	if ext.WarrantyEnd != nil {
		fields["warranty_end"] = ext.WarrantyEnd.Format(time.DateOnly)
	}
	if ext.Metadata != nil {
		fields["metadata"] = ext.Metadata
	}
	return fields
}

// rehydrateExtraction reconstructs an entity.Extraction from a stored
// candidate_fields map and the raw extraction string. It is the inverse of
// candidateFields for the fields stored in candidate_fields; Confidence is
// not rehydrated because it lives on a separate column on the review row
// (callers pass it through to CommitCandidate / Document when needed).
// Missing keys are treated as absent (nil pointer / zero value).
func rehydrateExtraction(fields map[string]any, raw string) entity.Extraction {
	ext := entity.Extraction{RawPayload: raw}

	if v, ok := fields["classification"].(string); ok && v != "" {
		ext.Classification = v
	}
	if v, ok := fields["brand"].(string); ok && v != "" {
		ext.Brand = &v
	}
	if v, ok := fields["model"].(string); ok && v != "" {
		ext.Model = &v
	}
	if v, ok := fields["serial_number"].(string); ok && v != "" {
		ext.SerialNumber = &v
	}
	if v, ok := fields["price"].(string); ok && v != "" {
		ext.Price = &v
	}
	if v, ok := fields["currency"].(string); ok && v != "" {
		ext.Currency = &v
	}
	if v, ok := fields["purchase_date"].(string); ok && v != "" {
		if t, err := time.Parse(time.DateOnly, v); err == nil {
			ext.PurchaseDate = &t
		}
	}
	if v, ok := fields["warranty_end"].(string); ok && v != "" {
		if t, err := time.Parse(time.DateOnly, v); err == nil {
			ext.WarrantyEnd = &t
		}
	}
	if v, ok := fields["metadata"].(map[string]any); ok {
		ext.Metadata = v
	}
	return ext
}
