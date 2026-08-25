package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-core/identity"
)

// ErrTooLarge is returned when the upload exceeds the configured size limit.
var ErrTooLarge = errors.New("file too large")

// ErrExtraction is returned when LLM extraction or parsing fails.
var ErrExtraction = errors.New("extraction failed")

// Service ingests uploaded documents: size check, storage, LLM extraction,
// identity resolution, and atomic asset+document persistence.
type Service struct {
	factory   data.RepoFactory
	extractor data.Extractor
	storage   data.FileStorage
	maxBytes  int64
}

// New constructs a Service. maxBytes is the maximum accepted upload size in
// bytes; uploads strictly larger are rejected with ErrTooLarge.
func New(factory data.RepoFactory, extractor data.Extractor, storage data.FileStorage, maxBytes int64) *Service {
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
// Source from the bytes.
func (s *Service) Process(ctx context.Context, filename string, payload []byte, contentType string) (data.Asset, error) {
	if int64(len(payload)) > s.maxBytes {
		return data.Asset{}, ErrTooLarge
	}

	source, err := s.storage.Put(ctx, payload)
	if err != nil {
		return data.Asset{}, err
	}

	// The repository is the source of truth for the persisted Source ID (it
	// may assign its own, as the assets repo does); the returned record is
	// used so doc.SourceID references a row that actually exists.
	source, err = s.factory.SourceRepo().Create(ctx, source)
	if err != nil {
		return data.Asset{}, err
	}

	raw, err := s.extractor.Extract(ctx, contentType, payload)
	if err != nil {
		return data.Asset{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	ext, err := data.ParseExtraction(raw)
	if err != nil {
		return data.Asset{}, fmt.Errorf("%w: %w", ErrExtraction, err)
	}

	var resolved data.Asset
	err = s.factory.InTransaction(ctx, func(ctx context.Context) error {
		txFactory, ok := data.RepoFactoryFromContext(ctx)
		if !ok {
			return errors.New("ingest: no tx factory in context")
		}
		a, _, err := identity.Resolve(ctx, txFactory.AssetRepo(), ext)
		if err != nil {
			return err
		}
		resolved = a

		doc := data.Document{
			SourceID:        source.ID,
			AssetID:         a.ID,
			DocType:         ext.Classification,
			ExtractedFields: extractionFields(ext),
			RawExtraction:   ext.RawPayload,
		}
		if _, err := txFactory.DocumentRepo().Create(ctx, doc); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return data.Asset{}, err
	}
	return resolved, nil
}

// extractionFields flattens the extraction into a non-nil map containing a
// key only for each present value.
func extractionFields(ext data.Extraction) map[string]any {
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
