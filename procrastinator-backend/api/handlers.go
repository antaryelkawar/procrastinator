package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/identity"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/infra/filestorage"
)

// findAssetScoped returns the single asset with id visible to the user under
// the scope access rule, or nil when the user cannot see it (unknown or
// another user's assets both surface as not-found).
func (s *Server) findAssetScoped(ctx context.Context, tid, id string) (*entity.Asset, error) {
	assets, err := s.factory.Assets.List(ctx, repo.Owner(tid), repo.Where("id", "=", id))
	if err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, nil
	}
	a := assets[0]
	return &a, nil
}

// handleUpload processes a multipart document upload: it enforces the size
// limit, reads the "file" part, resolves the optional owner household scope,
// and runs the full ingest pipeline.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}

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

	// Resolve the optional owner-household scope. Absent or blank means a
	// personal upload (ownerHH stays nil). When present, the requester must be
	// a member of that household.
	ownerHH := (*string)(nil)
	if hh := strings.TrimSpace(r.FormValue("owner_household_id")); hh != "" {
		member, err := s.isHouseholdMember(r.Context(), tid, hh)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !member {
			httpx.WriteError(w, http.StatusForbidden, "not a member of the household")
			return
		}
		ownerHH = &hh
	}

	asset, err := s.svc.Process(r.Context(), header.Filename, payload, header.Header.Get("Content-Type"), ownerHH)
	if err != nil {
		s.writeProcessError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAssetJSON(asset))
}

// isHouseholdMember reports whether userID is a member of householdID.
func (s *Server) isHouseholdMember(ctx context.Context, userID, householdID string) (bool, error) {
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

// handleListAssets returns all assets visible to the requesting user under the
// scope access rule.
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}
	assets, err := s.factory.Assets.List(r.Context(), repo.Owner(tid))
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

// handleGetAsset returns a single asset by ID when the requesting user can
// see it under the scope access rule; an asset the user can't see (unknown or
// another user's) fails with 404.
func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "assetId")

	asset, err := s.findAssetScoped(r.Context(), tid, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	}
	if asset == nil {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAssetJSON(*asset))
}

// handleListDocuments returns all documents attached to one asset. The asset
// is looked up first so that unknown or another user's assets fail with 404
// instead of an empty list.
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}
	id := chi.URLParam(r, "assetId")

	if asset, err := s.findAssetScoped(r.Context(), tid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	} else if asset == nil {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
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
