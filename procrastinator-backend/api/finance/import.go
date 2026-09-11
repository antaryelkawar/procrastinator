package finance

import (
	"context"
	"net/http"
	"strings"

	"github.com/oapi-codegen/runtime"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
)

// CreateImportBatch processes POST /api/users/{userId}/finance/import-batches:
// it binds the generated multipart body, reads the "file" part and the
// "account_id" field, and runs the statement import pipeline via the statement
// service. A 201 response carries the created batch with its parsed lines and
// source metadata.
func (s *Service) CreateImportBatch(ctx context.Context, request gen.CreateImportBatchRequestObject) (gen.CreateImportBatchResponseObject, error) {
	var body gen.CreateImportBatchMultipartBody
	if err := runtime.BindMultipart(&body, *request.Body); err != nil {
		if httpx.IsMaxBytesErr(err) {
			return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, httpx.NewAPIError(http.StatusBadRequest, "malformed multipart body")
	}

	accountID := strings.TrimSpace(body.AccountId)
	if accountID == "" {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "missing or blank account_id")
	}

	filename := body.File.Filename()
	payload, err := body.File.Bytes()
	if err != nil {
		if httpx.IsMaxBytesErr(err) {
			return nil, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "failed to read upload")
	}
	if len(payload) == 0 {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "missing file field")
	}

	batch, lines, err := s.statement.Upload(ctx, accountID, filename, payload)
	if err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	// Defensive: Upload persisted the batch together with this source, so a
	// lookup failure here is an invariant violation.
	src, err := s.factory.Sources.Get(ctx, batch.SourceID, repo.Owner(request.UserId))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.CreateImportBatch201JSONResponse(ToImportBatch(batch, lines, src)), nil
}

// ListImportBatches returns all of the requesting user's import batches
// (without their lines), joined with their source metadata. The result is
// never nil.
func (s *Service) ListImportBatches(ctx context.Context, request gen.ListImportBatchesRequestObject) (gen.ListImportBatchesResponseObject, error) {
	batches, err := s.statement.ListBatches(ctx)
	if err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	out := make([]gen.ImportBatch, 0, len(batches))
	for _, b := range batches {
		src, err := s.factory.Sources.Get(ctx, b.SourceID, repo.Owner(request.UserId))
		if err != nil {
			status, msg := httpx.MapStatementError(err)
			return nil, httpx.NewAPIError(status, msg)
		}
		out = append(out, ToImportBatch(b, nil, src))
	}
	return gen.ListImportBatches200JSONResponse(out), nil
}

// GetImportBatch returns one import batch with all of its parsed lines and
// source metadata. Unknown or another user's IDs yield 404.
func (s *Service) GetImportBatch(ctx context.Context, request gen.GetImportBatchRequestObject) (gen.GetImportBatchResponseObject, error) {
	batch, lines, err := s.statement.GetBatch(ctx, request.Id)
	if err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	src, err := s.factory.Sources.Get(ctx, batch.SourceID, repo.Owner(request.UserId))
	if err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.GetImportBatch200JSONResponse(ToImportBatch(batch, lines, src)), nil
}

// CommitImportBatch processes POST /api/users/{userId}/finance/import-batches/{id}/commit:
// it commits a preview batch, creating a ledger movement for every valid line,
// and returns the commit summary.
func (s *Service) CommitImportBatch(ctx context.Context, request gen.CommitImportBatchRequestObject) (gen.CommitImportBatchResponseObject, error) {
	summary, err := s.statement.Commit(ctx, request.Id)
	if err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.CommitImportBatch200JSONResponse(gen.CommitSummary{Created: summary.Created, Skipped: summary.Skipped}), nil
}

// DiscardImportBatch processes POST /api/users/{userId}/finance/import-batches/{id}/discard:
// it transitions a preview batch to the discarded (terminal) state and returns
// the discarded batch with its lines and source metadata.
func (s *Service) DiscardImportBatch(ctx context.Context, request gen.DiscardImportBatchRequestObject) (gen.DiscardImportBatchResponseObject, error) {
	if _, err := s.statement.Discard(ctx, request.Id); err != nil {
		status, msg := httpx.MapStatementError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	// Re-read the full batch for the response body. Defensive: Discard
	// succeeded, so the batch (and its source) must still be readable.
	batch, lines, err := s.statement.GetBatch(ctx, request.Id)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	src, err := s.factory.Sources.Get(ctx, batch.SourceID, repo.Owner(request.UserId))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.DiscardImportBatch200JSONResponse(ToImportBatch(batch, lines, src)), nil
}
