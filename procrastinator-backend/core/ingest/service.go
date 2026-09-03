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

// Service ingests uploaded documents: size check, storage, LLM extraction,
// identity resolution, and atomic asset+document persistence.
type Service struct {
	factory   *repo.Factory
	extractor repo.Extractor
	storage   repo.FileStorage
	maxBytes  int64
}

// New constructs a Service. maxBytes is the maximum accepted upload size in
// bytes; uploads strictly larger are rejected with ErrTooLarge.
func New(factory *repo.Factory, extractor repo.Extractor, storage repo.FileStorage, maxBytes int64) *Service {
	return &Service{
		factory:   factory,
		extractor: extractor,
		storage:   storage,
		maxBytes:  maxBytes,
	}
}

// Process runs the ingest pipeline for one upload: reject oversize uploads,
// store the bytes, persist the Source row (before the transaction, so it
// survives later failures), call the LLM extractor, parse the payload, then
// atomically resolve the asset identity and create the document. The
// filename is accepted for the API contract but the storage derives the
// Source from the bytes. ownerHouseholdID fences the resolution and stamps
// the source + document to a household scope (nil = personal).
func (s *Service) Process(ctx context.Context, filename string, payload []byte, contentType string, ownerHouseholdID *string) (entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, err
	}

	// 1. Size check
	if int64(len(payload)) > s.maxBytes {
		return entity.Asset{}, ErrTooLarge
	}

	// 2. Storage
	source, err := s.storage.Put(ctx, payload)
	if err != nil {
		return entity.Asset{}, err
	}

	// 3. Persist source (before tx, survives later failures)
	source.OwnerHouseholdID = ownerHouseholdID
	source, err = s.factory.Sources.Create(ctx, source, repo.Owner(tid))
	if err != nil {
		return entity.Asset{}, err
	}

	// 4. LLM extraction
	raw, err := s.extractor.Extract(ctx, contentType, payload)
	if err != nil {
		return entity.Asset{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	// 5. Parse
	ext, err := parse.ParseExtraction(raw)
	if err != nil {
		return entity.Asset{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	// 6. Transaction: resolve identity + create document
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
		}
		_, err = repos.Documents.Create(ctx, doc, repo.Owner(tid))
		return err
	})
	if err != nil {
		return entity.Asset{}, err
	}
	return resolved, nil
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
