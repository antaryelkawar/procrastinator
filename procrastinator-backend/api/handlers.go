package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/identity"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/infra/filestorage"
)

// handleUpload processes a multipart document upload: it enforces the size
// limit, reads the "file" part, and runs the full ingest pipeline.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes)

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

	asset, err := s.svc.Process(r.Context(), header.Filename, payload, header.Header.Get("Content-Type"))
	if err != nil {
		s.writeProcessError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAssetJSON(asset))
}

// writeProcessError maps an ingest service error onto the HTTP status
// contract. Checks run in priority order: the domain sentinels first, then
// the transport-level fallback.
func (s *Server) writeProcessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ingest.ErrTooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
	case errors.Is(err, filestorage.ErrUnsupportedType):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported file type")
	case errors.Is(err, ingest.ErrExtraction):
		httpx.WriteError(w, http.StatusBadGateway, "extraction failed")
	case errors.Is(err, identity.ErrNoIdentity):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no usable identity in document")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// handleListAssets returns all assets visible to the requesting tenant.
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	tid, ok := tenantFromCtx(w, r.Context())
	if !ok {
		return
	}
	assets, err := s.factory.Assets.List(r.Context(), repo.Tenant(tid))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset list: internal error")
		return
	}

	out := make([]assetJSON, 0, len(assets))
	for _, a := range assets {
		out = append(out, toAssetJSON(a))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// handleGetAsset returns a single asset by ID.
func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := tenantFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	asset, err := s.factory.Assets.Get(r.Context(), id, repo.Tenant(tid))
	if errors.Is(err, repo.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAssetJSON(asset))
}

// handleListDocuments returns all documents attached to one asset. The asset
// is looked up first so that unknown or foreign-tenant assets fail with 404
// instead of an empty list.
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	tid, ok := tenantFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	if _, err := s.factory.Assets.Get(r.Context(), id, repo.Tenant(tid)); errors.Is(err, repo.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	}

	docs, err := s.listDocumentsWithSource(r.Context(), tid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "document list: internal error")
		return
	}

	out := make([]documentJSON, 0, len(docs))
	for _, d := range docs {
		out = append(out, toDocumentJSON(d))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// isMaxBytesErr reports whether err (or any error in its chain) is an
// *http.MaxBytesError.
func isMaxBytesErr(err error) bool {
	var mbErr *http.MaxBytesError
	return errors.As(err, &mbErr)
}
