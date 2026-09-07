package api

import (
	"context"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/oapi-codegen/runtime"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/core/processing"
)

// addItem is one unit of work for the unified add endpoint: either an
// uploaded file or a pasted-text item.
type addItem struct {
	filename    string
	payload     []byte
	contentType string
	isText      bool
}

// AddItems handles the unified add endpoint (POST /api/users/{userId}/add).
// It binds the multipart body, normalizes the files and optional pasted text
// into an ordered list of items, validates the statement precondition, and
// runs each item through the statement import pipeline or the document
// processing pipeline. The response is a uniform per-item outcome array:
// per-item failures are reported as "failed" outcomes (a 200 carrying the
// array), and only the whole-request statement precondition surfaces as a
// 400.
func (s *Server) AddItems(ctx context.Context, request gen.AddItemsRequestObject) (gen.AddItemsResponseObject, error) {
	var body gen.AddItemsMultipartBody
	if err := runtime.BindMultipart(&body, *request.Body); err != nil {
		if isMaxBytesErr(err) {
			return nil, newAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, newAPIError(http.StatusBadRequest, "malformed multipart body")
	}

	// Normalize the request into an ordered list of items: one entry per
	// uploaded file (input order preserved, empty files skipped) plus a single
	// pasted-text item when non-blank text is supplied.
	var items []addItem
	if body.Files != nil {
		for _, f := range *body.Files {
			payload, err := f.Bytes()
			if err != nil {
				if isMaxBytesErr(err) {
					return nil, newAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
				}
				return nil, newAPIError(http.StatusBadRequest, "failed to read upload")
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
		return nil, newAPIError(http.StatusBadRequest, "nothing to add")
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
			return nil, newAPIError(http.StatusBadRequest, "statement items require an account_id")
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
func (s *Server) processAddItem(ctx context.Context, accountID string, it addItem) gen.AddItemOutcome {
	if isStatementItem(it.filename, it.contentType) {
		batch, _, err := s.statement.Upload(ctx, accountID, it.filename, it.payload)
		if err != nil {
			_, msg := mapStatementError(err)
			return gen.AddItemOutcome{Kind: gen.Failed, Reason: &msg}
		}
		id := batch.ID
		return gen.AddItemOutcome{Kind: gen.StatementPreview, ImportBatchId: &id}
	}

	outcome, err := s.svc.Process(ctx, processing.Input{
		Filename:    it.filename,
		Payload:     it.payload,
		ContentType: it.contentType,
		Text:        it.isText,
	})
	if err != nil {
		_, msg := mapIngestError(err)
		return gen.AddItemOutcome{Kind: gen.Failed, Reason: &msg}
	}
	return mapAddOutcome(outcome)
}

// mapAddOutcome maps a successful processing outcome to the uniform per-item
// outcome.
func mapAddOutcome(o processing.Outcome) gen.AddItemOutcome {
	switch o.Kind {
	case processing.OutcomeCommitted:
		id := o.Asset.ID
		return gen.AddItemOutcome{Kind: gen.AssetCommitted, AssetId: &id}
	case processing.OutcomeHeldForReview:
		id := o.Review.ID
		return gen.AddItemOutcome{Kind: gen.HeldForReview, ReviewId: &id}
	case processing.OutcomeDuplicate:
		assetID := o.Duplicate.AssetID
		docID := o.Duplicate.DocumentID
		deleted := o.Duplicate.AssetDeleted
		return gen.AddItemOutcome{Kind: gen.Duplicate, DuplicateAssetId: &assetID, DuplicateDocumentId: &docID, AssetDeleted: &deleted}
	case processing.OutcomeFailed:
		reason := o.Reason
		if reason == "" {
			reason = "processing failed"
		}
		return gen.AddItemOutcome{Kind: gen.Failed, Reason: &reason}
	case processing.OutcomeStatement:
		// Edge case: a non-CSV item the LLM classified as a statement. The
		// add surface has no account selected for this item, so it cannot be
		// routed to the statement import pipeline.
		reason := "statement detected but no account was selected for this item"
		return gen.AddItemOutcome{Kind: gen.Failed, Reason: &reason}
	default:
		reason := "unknown outcome"
		return gen.AddItemOutcome{Kind: gen.Failed, Reason: &reason}
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
