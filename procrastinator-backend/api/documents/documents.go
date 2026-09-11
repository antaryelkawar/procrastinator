// Package documents holds the documents-section API handlers (list, reprocess,
// delete, pending-choice mutators + sweeper).
package documents

import (
	"context"
	"net/http"
	"strings"
	"time"

	"procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/processing"
)

// Service serves the documents-section endpoints (list, reprocess, keep,
// delete). It is the bounded-context service for the user's document library:
// it derives each document's processing status, applies the pending-choice
// lazy guard, and drives re-extraction through the processing service.
type Service struct {
	factory *repo.Factory
	svc     *processing.Service
}

// New constructs a Service.
func New(factory *repo.Factory, svc *processing.Service) *Service {
	return &Service{factory: factory, svc: svc}
}

// findDocumentScoped returns the single non-deleted document with id visible
// to the user under the scope access rule, or (nil, nil) when the user cannot
// see it (unknown or another user's document both surface as not-found).
func (s *Service) findDocumentScoped(ctx context.Context, tid, id string) (*entity.Document, error) {
	docs, err := s.factory.Documents.List(ctx, repo.Owner(tid), repo.Where("id", "=", id))
	if err != nil {
		return nil, err
	}
	for i := range docs {
		if docs[i].DeletedAt == nil {
			return &docs[i], nil
		}
	}
	return nil, nil
}

// joinSource returns the DocumentWithSource for d, joining in the source's
// filename and upload date (the list endpoint renders these).
func (s *Service) joinSource(ctx context.Context, tid string, d entity.Document) (entity.DocumentWithSource, error) {
	dw := entity.DocumentWithSource{Document: d}
	srcs, err := s.factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", d.SourceID))
	if err != nil {
		return entity.DocumentWithSource{}, err
	}
	if len(srcs) == 0 {
		// Unreachable when the document is visible (the source shares its
		// scope); fail closed rather than emit a partial row.
		return entity.DocumentWithSource{}, repo.ErrNotFound
	}
	dw.SourceFilename = srcs[0].Filename
	dw.SourceUploadedAt = srcs[0].UploadedAt
	return dw, nil
}

// pendingReviewSourceIDs returns the set of source_ids that have a pending
// (non-deleted) ingest review. A document whose source is in this set is
// in_review.
func (s *Service) pendingReviewSourceIDs(ctx context.Context, tid string) (map[string]bool, error) {
	reviews, err := s.factory.Reviews.List(ctx, repo.Owner(tid), repo.Where("state", "=", "pending"))
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(reviews))
	for _, r := range reviews {
		if r.DeletedAt == nil {
			set[r.SourceID] = true
		}
	}
	return set, nil
}

// deriveStatus derives a document's processing status:
//   - processed:  the document is linked to an asset (AssetID != "")
//   - in_review:  a pending ingest review exists for the document's source
//   - asset_less: no linked asset and no pending review
//
// failed is unreachable in the current code (no document row is created on
// extraction failure), so it is never derived here.
func deriveStatus(d entity.Document, pendingReview map[string]bool) gen.DocumentStatus {
	if d.AssetID != "" {
		return gen.DocumentStatusProcessed
	}
	if pendingReview[d.SourceID] {
		return gen.DocumentStatusInReview
	}
	return gen.DocumentStatusAssetLess
}

// ListDocuments returns the user's non-deleted documents with their source
// metadata and derived status, optionally filtered by status and by a
// case-insensitive source-filename substring (q).
//
// The D7a lazy guard (expired pending → keep_existing) is not applied here:
// the sweeper handles background resolution, and the status derivation is
// correct even without it (an expired pending doc still has its asset_id set,
// so it derives as "processed"). Write paths (reprocess, keep, delete) do
// apply the guard before mutating.
func (s *Service) ListDocuments(ctx context.Context, request gen.ListDocumentsRequestObject) (gen.ListDocumentsResponseObject, error) {
	tid := request.UserId

	docs, err := s.factory.Documents.List(ctx, repo.Owner(tid), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document list: internal error")
	}
	pendingReview, err := s.pendingReviewSourceIDs(ctx, tid)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document list: internal error")
	}

	var statusFilter gen.DocumentStatus
	if request.Params.Status != nil {
		statusFilter = gen.DocumentStatus(*request.Params.Status)
	}
	q := ""
	if request.Params.Q != nil {
		q = *request.Params.Q
	}

	out := make([]gen.Document, 0, len(docs))
	for _, d := range docs {
		if d.DeletedAt != nil {
			continue // soft-deleted documents are hidden
		}
		status := deriveStatus(d, pendingReview)
		if statusFilter != "" && status != statusFilter {
			continue
		}
		dw, err := s.joinSource(ctx, tid, d)
		if err != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document list: internal error")
		}
		if q != "" && !strings.Contains(strings.ToLower(dw.SourceFilename), strings.ToLower(q)) {
			continue
		}
		out = append(out, dto.ToDocument(dw, status))
	}
	return gen.ListDocuments200JSONResponse(out), nil
}

// applyLazyGuard resolves an expired pending choice on doc (mutating and
// persisting it) before a mutator runs, so the caller always acts on current
// state. A pending choice whose expires_at is in the past is resolved to
// keep_existing; an unexpired or absent choice is left untouched.
func (s *Service) applyLazyGuard(ctx context.Context, doc *entity.Document) (*entity.Document, error) {
	if doc.PendingChoice != nil && doc.PendingChoice.State == "pending" && doc.PendingChoice.ExpiresAt.Before(time.Now()) {
		doc.PendingChoice.State = "resolved"
		doc.PendingChoice.Outcome = "keep_existing"
		updated, err := s.factory.Documents.Update(ctx, *doc, repo.Owner(doc.OwnerID))
		if err != nil {
			return nil, err
		}
		doc = &updated
		return doc, nil
	}
	return doc, nil
}

// ReprocessDocument re-runs extraction for the document's source (with an
// optional comment as the directive) and updates the document with the outcome.
// It refuses (409) when the document is already in_review (a reprocess is
// already in-flight for that source).
func (s *Service) ReprocessDocument(ctx context.Context, request gen.ReprocessDocumentRequestObject) (gen.ReprocessDocumentResponseObject, error) {
	tid := request.UserId

	doc, err := s.findDocumentScoped(ctx, tid, request.Id)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	if doc == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "document not found")
	}
	doc, err = s.applyLazyGuard(ctx, doc)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}

	// In-flight guard: a pending review for this source means a reprocess is
	// already running (or the upload is still held for review).
	pendingReview, err := s.pendingReviewSourceIDs(ctx, tid)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	if pendingReview[doc.SourceID] {
		return nil, httpx.NewAPIError(http.StatusConflict, "document is already in-flight")
	}

	// Optional comment → extraction directive, persisted as user_directive.
	directive := ""
	if request.Body != nil && request.Body.Comment != nil {
		directive = *request.Body.Comment
	}
	doc.UserDirective = directive
	updated, err := s.factory.Documents.Update(ctx, *doc, repo.Owner(tid))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	doc = &updated

	// Re-run extraction for the existing source.
	outcome, err := s.svc.Reprocess(ctx, doc.SourceID, directive, doc.OwnerHouseholdID)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "reprocess: internal error")
	}

	switch outcome.Kind {
	case processing.OutcomeCommitted:
		// The reprocess resolved to (possibly a new/merged) asset: point the
		// document at it. The document is now processed.
		if outcome.Asset != nil {
			doc.AssetID = outcome.Asset.ID
		}
		updated, uerr := s.factory.Documents.Update(ctx, *doc, repo.Owner(tid))
		if uerr != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
		}
		doc = &updated
		status := gen.DocumentStatusProcessed
		dw, jerr := s.joinSource(ctx, tid, *doc)
		if jerr != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
		}
		return gen.ReprocessDocument202JSONResponse(dto.ToDocument(dw, status)), nil
	case processing.OutcomeHeldForReview:
		// A new review was created; the document is in_review (asset-less).
		dw, jerr := s.joinSource(ctx, tid, *doc)
		if jerr != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
		}
		return gen.ReprocessDocument202JSONResponse(dto.ToDocument(dw, gen.DocumentStatusInReview)), nil
	default:
		// OutcomeFailed / OutcomeStatement / OutcomeDuplicate: the document
		// remains asset-less (no asset to link, no review created).
		dw, jerr := s.joinSource(ctx, tid, *doc)
		if jerr != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
		}
		return gen.ReprocessDocument202JSONResponse(dto.ToDocument(dw, gen.DocumentStatusAssetLess)), nil
	}
}

// KeepDocument resolves a document's pending choice to keep_existing: the
// duplicate is kept as-is (the existing document is retained) and the choice
// is marked resolved.
func (s *Service) KeepDocument(ctx context.Context, request gen.KeepDocumentRequestObject) (gen.KeepDocumentResponseObject, error) {
	tid := request.UserId

	doc, err := s.findDocumentScoped(ctx, tid, request.Id)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	if doc == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "document not found")
	}
	doc, err = s.applyLazyGuard(ctx, doc)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	if doc.PendingChoice != nil {
		doc.PendingChoice.State = "resolved"
		doc.PendingChoice.Outcome = "keep_existing"
		updated, uerr := s.factory.Documents.Update(ctx, *doc, repo.Owner(tid))
		if uerr != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
		}
		doc = &updated
	}
	dw, err := s.joinSource(ctx, tid, *doc)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	pendingReview, err := s.pendingReviewSourceIDs(ctx, tid)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	status := deriveStatus(*doc, pendingReview)
	return gen.KeepDocument200JSONResponse(dto.ToDocument(dw, status)), nil
}

// DeleteDocument soft-deletes a document and detaches it from its asset (the
// asset row is retained; the FK ON DELETE SET NULL is mirrored by explicitly
// clearing asset_id on the document).
func (s *Service) DeleteDocument(ctx context.Context, request gen.DeleteDocumentRequestObject) (gen.DeleteDocumentResponseObject, error) {
	tid := request.UserId

	doc, err := s.findDocumentScoped(ctx, tid, request.Id)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	if doc == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "document not found")
	}
	doc, err = s.applyLazyGuard(ctx, doc)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	now := time.Now()
	doc.DeletedAt = &now
	doc.AssetID = ""
	_, err = s.factory.Documents.Update(ctx, *doc, repo.Owner(tid), repo.Set("asset_id", nil))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document: internal error")
	}
	return gen.DeleteDocument204Response{}, nil
}
