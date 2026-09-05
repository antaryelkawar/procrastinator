package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/parse"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/identity"
)

// ErrTooLarge is returned when the upload exceeds the configured size limit.
var ErrTooLarge = errors.New("file too large")

// ErrExtraction is returned when LLM extraction or parsing fails.
var ErrExtraction = errors.New("extraction failed")

// Result is the outcome of a Process call. Exactly one of Committed or
// Review is non-nil: Committed is set when the confidence gate passes
// (auto-commit path), Review is set when it does not (held-for-review path).
type Result struct {
	Committed *entity.Asset
	Review    *entity.IngestReview
}

// Service ingests uploaded documents: size check, storage, LLM extraction,
// confidence gate, and either atomic asset+document persistence (commit) or
// a pending review (hold).
type Service struct {
	factory   *repo.Factory
	extractor repo.Extractor
	storage   repo.FileStorage
	maxBytes  int64
	threshold float64
	reviewer  repo.Reviewer
}

// New constructs a Service. maxBytes is the maximum accepted upload size in
// bytes; uploads strictly larger are rejected with ErrTooLarge. threshold is
// the minimum confidence required for auto-commit (default 0.7). reviewer
// handles the hold path when confidence is below threshold or absent.
func New(factory *repo.Factory, extractor repo.Extractor, storage repo.FileStorage, maxBytes int64, threshold float64, reviewer repo.Reviewer) *Service {
	return &Service{
		factory:   factory,
		extractor: extractor,
		storage:   storage,
		maxBytes:  maxBytes,
		threshold: threshold,
		reviewer:  reviewer,
	}
}

// Process runs the ingest pipeline for one upload: reject oversize uploads,
// store the bytes, persist the Source row (before the transaction, so it
// survives later failures), call the LLM extractor, parse the payload, then
// apply the confidence gate: if confidence >= threshold, atomically resolve
// the asset identity and create the document (commit); otherwise hold the
// upload as a pending review. The filename is accepted for the API contract
// but the storage derives the Source from the bytes. ownerHouseholdID fences
// the resolution and stamps the source + document to a household scope
// (nil = personal).
func (s *Service) Process(ctx context.Context, filename string, payload []byte, contentType string, ownerHouseholdID *string) (Result, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return Result{}, err
	}

	// 1. Size check
	if int64(len(payload)) > s.maxBytes {
		return Result{}, ErrTooLarge
	}

	// 2. Storage
	source, err := s.storage.Put(ctx, payload)
	if err != nil {
		return Result{}, err
	}

	// 3. Persist source (before tx, survives later failures)
	source.OwnerHouseholdID = ownerHouseholdID
	source, err = s.factory.Sources.Create(ctx, source, repo.Owner(tid))
	if err != nil {
		return Result{}, err
	}

	// 4. LLM extraction
	raw, err := s.extractor.Extract(ctx, contentType, payload)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	// 5. Parse
	ext, err := parse.ParseExtraction(raw)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	// 6. Confidence gate: commit if confidence is present and >= threshold.
	commit := ext.Confidence != nil && *ext.Confidence >= s.threshold

	if commit {
		// Commit path: identity.Resolve + Documents.Create
		var resolved entity.Asset
		err = s.factory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
			a, _, err := identity.Resolve(ctx, repos.Assets, ext, ownerHouseholdID)
			if err != nil {
				return err
			}
			resolved = a

			doc := entity.Document{
				SourceID:         source.ID,
				AssetID:          a.ID,
				DocType:          ext.Classification,
				ExtractedFields:  extractionFields(ext),
				RawExtraction:    ext.RawPayload,
				OwnerHouseholdID: ownerHouseholdID,
				Confidence:       ext.Confidence,
			}
			_, err = repos.Documents.Create(ctx, doc, repo.Owner(tid))
			return err
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Committed: &resolved}, nil
	}

	// Hold path: create pending review via the reviewer.
	rev, err := s.reviewer.Hold(ctx, repo.HoldInput{
		Extraction:       ext,
		SourceID:         source.ID,
		OwnerHouseholdID: ownerHouseholdID,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Review: &rev}, nil
}

// extractionFields flattens the extraction into a non-nil map containing a
// key only for each present value.
func extractionFields(ext entity.Extraction) map[string]any {
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
