package add

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/oapi-codegen/runtime"

	"procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/processing"
	"procrastinator-backend/core/statement"
)

// Service serves the add endpoints (unified add + document upload).
type Service struct {
	svc       *processing.Service
	factory   *repo.Factory
	statement *statement.Service
}

// New constructs a Service.
func New(svc *processing.Service, factory *repo.Factory, statement *statement.Service) *Service {
	return &Service{svc: svc, factory: factory, statement: statement}
}

// addItem is one unit of work for the unified add endpoint: either an
// uploaded file or a pasted-text item.
type addItem struct {
	filename    string
	payload     []byte
	contentType string
	isText      bool
}

// UploadDocument processes a multipart document upload: it binds the generated
// multipart body, reads the "file" part, derives the file MIME from its
// extension, resolves the optional owner household scope, pre-checks for a
// content-hash duplicate, and either runs the full ingest pipeline or returns
// a 409 DuplicateReport for the caller to prompt the user (design D4/D7a).
func (s *Service) UploadDocument(ctx context.Context, request gen.UploadDocumentRequestObject) (gen.UploadDocumentResponseObject, error) {
	var body gen.UploadDocumentMultipartBody
	if err := runtime.BindMultipart(&body, *request.Body); err != nil {
		if httpx.IsMaxBytesErr(err) {
			return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, httpx.NewAPIError(http.StatusBadRequest, "malformed multipart body")
	}

	file := body.File
	filename := file.Filename()
	payload, err := file.Bytes()
	if err != nil {
		if httpx.IsMaxBytesErr(err) {
			return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "failed to read upload")
	}
	if len(payload) == 0 {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "missing file field")
	}

	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Optional note (user directive).
	directive := ""
	if body.Note != nil {
		directive = strings.TrimSpace(*body.Note)
	}

	// Resolve the optional owner-household scope.
	ownerHH := (*string)(nil)
	if body.OwnerHouseholdId != nil {
		hh := strings.TrimSpace(*body.OwnerHouseholdId)
		if hh != "" {
			member, err := s.isHouseholdMember(ctx, request.UserId, hh)
			if err != nil {
				return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
			}
			if !member {
				return nil, httpx.NewAPIError(http.StatusForbidden, "not a member of the household")
			}
			ownerHH = &hh
		}
	}

	// Pre-check for duplicate content hash. If the hash matches an existing
	// source in the caller's scope, store the new source, create a pending
	// document, and return 409 with the DuplicateReport (design D4/D7a).
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	dup, isDup, err := processing.DedupeSource(ctx, s.factory.Sources, s.factory.Documents, s.factory.Assets, hash)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	if isDup {
		return s.handleDuplicateUpload(ctx, request.UserId, filename, payload, contentType, ownerHH, directive, dup.Duplicate)
	}

	// Non-duplicate path: run the full pipeline with the directive.
	outcome, err := s.svc.Process(ctx, processing.Input{
		Filename:         filename,
		Payload:          payload,
		ContentType:      contentType,
		OwnerHouseholdID: ownerHH,
		Directive:        directive,
	})
	if err != nil {
		status, msg := httpx.MapIngestError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	switch outcome.Kind {
	case processing.OutcomeCommitted:
		return gen.UploadDocument201JSONResponse(dto.ToAsset(*outcome.Asset)), nil
	case processing.OutcomeHeldForReview:
		d, err := dto.ToReview(s.factory, ctx, request.UserId, *outcome.Review)
		if err != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
		}
		return gen.UploadDocument202JSONResponse(d), nil
	case processing.OutcomeDuplicate:
		// This should not be reached (handled above), but handle it defensively.
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "unexpected duplicate outcome")
	case processing.OutcomeFailed:
		return nil, httpx.NewAPIError(http.StatusBadGateway, "extraction failed")
	case processing.OutcomeStatement:
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "statement routing not yet supported")
	default:
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "unknown outcome")
	}
}

// handleDuplicateUpload stores the duplicate source, creates a pending document
// with pending_choice (design D7a), and returns 409 with the DuplicateReport.
func (s *Service) handleDuplicateUpload(ctx context.Context, userID, filename string, payload []byte, contentType string, ownerHH *string, directive string, dup *processing.Duplicate) (gen.UploadDocumentResponseObject, error) {
	// Store the new source for the duplicate upload.
	src, err := s.svc.StoreSource(ctx, processing.Input{
		Filename:         filename,
		Payload:          payload,
		ContentType:      contentType,
		OwnerHouseholdID: ownerHH,
		Directive:        directive,
	})
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}

	// Create the pending document with pending_choice (D7a: creation only,
	// never resolves — mutators live in task 3.4).
	now := time.Now()
	expiresAt := now.Add(10 * time.Minute)
	doc := entity.Document{
		SourceID:         src.ID,
		AssetID:          dup.AssetID,
		DocType:          "other",
		UserDirective:    directive,
		OwnerHouseholdID: ownerHH,
		PendingChoice: &entity.PendingChoice{
			State:     "pending",
			Outcome:   "",
			CreatedAt: now,
			ExpiresAt: expiresAt,
		},
	}
	pendingDoc, err := s.factory.Documents.Create(ctx, doc, repo.Owner(userID))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}

	// Build the DuplicateReport (design D4).
	report := gen.DuplicateReport{
		Code:                     gen.DuplicateReportCodeDuplicate,
		ExistingSourceFilename:   dup.SourceFilename,
		ExistingSourceUploadedAt: dup.SourceUploadedAt,
	}
	if dup.DocumentID != "" {
		report.ExistingDocumentId = &dup.DocumentID
	}
	if dup.AssetID != "" {
		report.ExistingAssetId = &dup.AssetID
	}
	report.Prompt.ReprocessUri = fmt.Sprintf("/api/users/%s/documents/%s/reprocess", userID, pendingDoc.ID)
	report.Prompt.KeepUri = fmt.Sprintf("/api/users/%s/documents/%s/keep", userID, pendingDoc.ID)
	report.Prompt.ExpiresAt = expiresAt
	report.Prompt.TimeoutToast = "no response — keeping existing document"

	return gen.UploadDocument409JSONResponse(report), nil
}

// isHouseholdMember reports whether userID is a member of householdID.
func (s *Service) isHouseholdMember(ctx context.Context, userID, householdID string) (bool, error) {
	hhs, err := s.factory.Households.HouseholdsForUser(ctx, userID, repo.Owner(userID))
	if err != nil {
		return false, err
	}
	for _, h := range hhs {
		if h == householdID {
			return true, nil
		}
	}
	return false, nil
}

// AddItems handles the unified add endpoint (POST /api/users/{userId}/add).
// It binds the multipart body, normalizes the files and optional pasted text
// into an ordered list of items, validates the statement precondition, and
// runs each item through the statement import pipeline or the document
// processing pipeline. The response is a uniform per-item outcome array:
// per-item failures are reported as "failed" outcomes (a 200 carrying the
// array), and only the whole-request statement precondition surfaces as a
// 400.
func (s *Service) AddItems(ctx context.Context, request gen.AddItemsRequestObject) (gen.AddItemsResponseObject, error) {
	var body gen.AddItemsMultipartBody
	if err := runtime.BindMultipart(&body, *request.Body); err != nil {
		if httpx.IsMaxBytesErr(err) {
			return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, httpx.NewAPIError(http.StatusBadRequest, "malformed multipart body")
	}

	// Normalize the request into an ordered list of items: one entry per
	// uploaded file (input order preserved, empty files skipped) plus a single
	// pasted-text item when non-blank text is supplied.
	var items []addItem
	if body.Files != nil {
		for _, f := range *body.Files {
			payload, err := f.Bytes()
			if err != nil {
				if httpx.IsMaxBytesErr(err) {
					return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
				}
				return nil, httpx.NewAPIError(http.StatusBadRequest, "failed to read upload")
			}
			if len(payload) == 0 {
				continue
			}
			filename := f.Filename()
			contentType := mime.TypeByExtension(filepath.Ext(filename))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			items = append(items, addItem{filename: filename, payload: payload, contentType: contentType, isText: false})
		}
	}
	if body.Text != nil && strings.TrimSpace(*body.Text) != "" {
		items = append(items, addItem{filename: "pasted.txt", payload: []byte(*body.Text), contentType: "text/plain", isText: true})
	}
	if len(items) == 0 {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "nothing to add")
	}

	// accountID scopes statement items; blank or absent means none.
	accountID := ""
	if body.AccountId != nil {
		accountID = strings.TrimSpace(*body.AccountId)
	}

	// Whole-request precondition (the only 400 for this endpoint): any
	// statement item requires an account_id. Per-item failures are reported
	// as "failed" outcomes instead.
	for _, it := range items {
		if isStatementItem(it.filename, it.contentType) && accountID == "" {
			return nil, httpx.NewAPIError(http.StatusBadRequest, "statement items require an account_id")
		}
	}

	// Produce exactly one outcome per item, preserving input order.
	outcomes := make([]gen.AddItemOutcome, 0, len(items))
	for _, it := range items {
		outcomes = append(outcomes, s.processAddItem(ctx, accountID, it))
	}
	return gen.AddItems200JSONResponse(outcomes), nil
}

// processAddItem runs a single add item through the appropriate pipeline and
// returns its uniform outcome.
func (s *Service) processAddItem(ctx context.Context, accountID string, it addItem) gen.AddItemOutcome {
	if isStatementItem(it.filename, it.contentType) {
		batch, _, err := s.statement.Upload(ctx, accountID, it.filename, it.payload)
		if err != nil {
			_, msg := httpx.MapStatementError(err)
			return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindFailed, Reason: &msg}
		}
		id := batch.ID
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindStatementPreview, ImportBatchId: &id}
	}

	outcome, err := s.svc.Process(ctx, processing.Input{
		Filename:    it.filename,
		Payload:     it.payload,
		ContentType: it.contentType,
		Text:        it.isText,
	})
	if err != nil {
		_, msg := httpx.MapIngestError(err)
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindFailed, Reason: &msg}
	}
	return mapAddOutcome(outcome)
}

// mapAddOutcome maps a successful processing outcome to the uniform per-item
// outcome.
func mapAddOutcome(o processing.Outcome) gen.AddItemOutcome {
	switch o.Kind {
	case processing.OutcomeCommitted:
		id := o.Asset.ID
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindAssetCommitted, AssetId: &id}
	case processing.OutcomeHeldForReview:
		id := o.Review.ID
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindHeldForReview, ReviewId: &id}
	case processing.OutcomeDuplicate:
		assetID := o.Duplicate.AssetID
		docID := o.Duplicate.DocumentID
		deleted := o.Duplicate.AssetDeleted
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindDuplicate, DuplicateAssetId: &assetID, DuplicateDocumentId: &docID, AssetDeleted: &deleted}
	case processing.OutcomeFailed:
		reason := o.Reason
		if reason == "" {
			reason = "processing failed"
		}
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindFailed, Reason: &reason}
	case processing.OutcomeStatement:
		// Edge case: a non-CSV item the LLM classified as a statement. The
		// add surface has no account selected for this item, so it cannot be
		// routed to the statement import pipeline.
		reason := "statement detected but no account was selected for this item"
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindFailed, Reason: &reason}
	default:
		reason := "unknown outcome"
		return gen.AddItemOutcome{Kind: gen.AddItemOutcomeKindFailed, Reason: &reason}
	}
}

// isStatementItem reports whether an add item should be routed to the
// statement import pipeline: a .csv file extension (case-insensitive) or a
// text/csv content type.
func isStatementItem(filename, contentType string) bool {
	if strings.EqualFold(filepath.Ext(filename), ".csv") {
		return true
	}
	return strings.HasPrefix(contentType, "text/csv")
}
