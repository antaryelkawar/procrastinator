package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/statement"
)

// createImportBatch processes POST /api/finance/import-batches: it enforces
// the statement size limit, reads the multipart "file" part and the
// "account_id" field, and runs the statement import pipeline via the
// statement service. A 201 response carries the created batch with its
// parsed lines and source metadata.
func (s *Server) createImportBatch(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.maxStatementBytes)
	if err := r.ParseMultipartForm(0); err != nil {
		if isMaxBytesErr(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "malformed multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer func() { _ = file.Close() }()

	payload, err := io.ReadAll(file)
	if err != nil {
		if isMaxBytesErr(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read upload")
		return
	}

	accountID := strings.TrimSpace(r.FormValue("account_id"))
	if accountID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing or blank account_id")
		return
	}

	batch, lines, err := s.statement.Upload(r.Context(), accountID, header.Filename, payload)
	if err != nil {
		writeImportError(w, err)
		return
	}

	// Defensive: Upload persisted the batch together with this source, so a
	// lookup failure here is an invariant violation.
	src, err := s.factory.Sources.Get(r.Context(), batch.SourceID, repo.Owner(tid))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toImportBatchJSON(batch, lines, src))
}

// listImportBatches returns all of the requesting user's import batches
// (without their lines), joined with their source metadata. The result is
// never nil.
func (s *Server) listImportBatches(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}

	batches, err := s.statement.ListBatches(r.Context())
	if err != nil {
		writeImportError(w, err)
		return
	}

	out := make([]importBatchJSON, 0, len(batches))
	for _, b := range batches {
		src, err := s.factory.Sources.Get(r.Context(), b.SourceID, repo.Owner(tid))
		if err != nil {
			writeImportError(w, err)
			return
		}
		out = append(out, toImportBatchJSON(b, nil, src))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// getImportBatch returns one import batch with all of its parsed lines and
// source metadata. Unknown or another user's IDs yield 404.
func (s *Server) getImportBatch(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	batch, lines, err := s.statement.GetBatch(r.Context(), id)
	if err != nil {
		writeImportError(w, err)
		return
	}

	src, err := s.factory.Sources.Get(r.Context(), batch.SourceID, repo.Owner(tid))
	if err != nil {
		writeImportError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toImportBatchJSON(batch, lines, src))
}

// commitImportBatch processes POST /api/finance/import-batches/{id}/commit:
// it commits a preview batch, creating a ledger movement for every valid
// line, and returns the commit summary.
func (s *Server) commitImportBatch(w http.ResponseWriter, r *http.Request) {
	if _, ok := userFromCtx(w, r.Context()); !ok {
		return
	}
	id := chi.URLParam(r, "id")

	summary, err := s.statement.Commit(r.Context(), id)
	if err != nil {
		writeImportError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, commitSummaryJSON{Created: summary.Created, Skipped: summary.Skipped})
}

// discardImportBatch processes POST /api/finance/import-batches/{id}/discard:
// it transitions a preview batch to the discarded (terminal) state and
// returns the discarded batch with its lines and source metadata.
func (s *Server) discardImportBatch(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	if _, err := s.statement.Discard(r.Context(), id); err != nil {
		writeImportError(w, err)
		return
	}

	// Re-read the full batch for the response body. Defensive: Discard
	// succeeded, so the batch (and its source) must still be readable.
	batch, lines, err := s.statement.GetBatch(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	src, err := s.factory.Sources.Get(r.Context(), batch.SourceID, repo.Owner(tid))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toImportBatchJSON(batch, lines, src))
}

// writeImportError maps core/statement and persistence sentinels onto the
// HTTP status contract (D11): ErrTooLarge -> 413, ErrUnsupportedType -> 415,
// ErrNoLines / ErrTooManyLines -> 422, ErrConflict -> 409,
// repo.ErrNotFound -> 404, user.ErrNoUser -> 401 (defensive), else 500.
func writeImportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, statement.ErrTooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
	case errors.Is(err, statement.ErrUnsupportedType):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported statement type")
	case errors.Is(err, statement.ErrNoLines):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "statement has no parseable lines")
	case errors.Is(err, statement.ErrTooManyLines):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "statement exceeds line limit")
	case errors.Is(err, statement.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflicting batch state")
	case errors.Is(err, repo.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, user.ErrNoUser):
		httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid user identity")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
