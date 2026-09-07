package api

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/oapi-codegen/runtime"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/processing"
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

// UploadDocument processes a multipart document upload: it binds the generated
// multipart body, reads the "file" part, derives the file MIME from its
// extension, resolves the optional owner household scope, and runs the full
// ingest pipeline.
func (s *Server) UploadDocument(ctx context.Context, request gen.UploadDocumentRequestObject) (gen.UploadDocumentResponseObject, error) {
	var body gen.UploadDocumentMultipartBody
	if err := runtime.BindMultipart(&body, *request.Body); err != nil {
		if isMaxBytesErr(err) {
			return nil, newAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, newAPIError(http.StatusBadRequest, "malformed multipart body")
	}

	file := body.File
	filename := file.Filename()
	payload, err := file.Bytes()
	if err != nil {
		if isMaxBytesErr(err) {
			return nil, newAPIError(http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		}
		return nil, newAPIError(http.StatusInternalServerError, "failed to read upload")
	}
	if len(payload) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "missing file field")
	}

	// Derive MIME from filename extension (the generated File type doesn't
	// expose Content-Type).
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Resolve the optional owner-household scope. Absent or blank means a
	// personal upload (ownerHH stays nil). When present, the requester must be
	// a member of that household.
	ownerHH := (*string)(nil)
	if body.OwnerHouseholdId != nil {
		hh := strings.TrimSpace(*body.OwnerHouseholdId)
		if hh != "" {
			member, err := s.isHouseholdMember(ctx, request.UserId, hh)
			if err != nil {
				return nil, newAPIError(http.StatusInternalServerError, "internal error")
			}
			if !member {
				return nil, newAPIError(http.StatusForbidden, "not a member of the household")
			}
			ownerHH = &hh
		}
	}

	outcome, err := s.svc.Process(ctx, processing.Input{
		Filename:         filename,
		Payload:          payload,
		ContentType:      contentType,
		OwnerHouseholdID: ownerHH,
	})
	if err != nil {
		status, msg := mapIngestError(err)
		return nil, newAPIError(status, msg)
	}
	switch outcome.Kind {
	case processing.OutcomeCommitted:
		return gen.UploadDocument201JSONResponse(toAsset(*outcome.Asset)), nil
	case processing.OutcomeHeldForReview:
		dto, err := s.toReview(ctx, request.UserId, *outcome.Review)
		if err != nil {
			return nil, newAPIError(http.StatusInternalServerError, "internal error")
		}
		return gen.UploadDocument202JSONResponse(dto), nil
	case processing.OutcomeDuplicate:
		// The document was already uploaded; return the linked asset as 201.
		// Fetch the asset by ID (the Duplicate struct only carries the ID).
		asset, err := s.findAssetScoped(ctx, request.UserId, outcome.Duplicate.AssetID)
		if err != nil {
			return nil, newAPIError(http.StatusInternalServerError, "internal error")
		}
		if asset == nil {
			// Asset may have been purged; fall back to 500.
			return nil, newAPIError(http.StatusInternalServerError, "duplicate asset not found")
		}
		return gen.UploadDocument201JSONResponse(toAsset(*asset)), nil
	case processing.OutcomeFailed:
		return nil, newAPIError(http.StatusBadGateway, "extraction failed")
	case processing.OutcomeStatement:
		// Statement routing is not yet supported in the upload endpoint contract.
		return nil, newAPIError(http.StatusInternalServerError, "statement routing not yet supported")
	default:
		return nil, newAPIError(http.StatusInternalServerError, "unknown outcome")
	}
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

// ListAssets returns all assets visible to the requesting user under the
// scope access rule.
func (s *Server) ListAssets(ctx context.Context, request gen.ListAssetsRequestObject) (gen.ListAssetsResponseObject, error) {
	opts := []repo.Option{repo.Owner(request.UserId)}
	includeDeleted := request.Params.IncludeDeleted != nil && *request.Params.IncludeDeleted
	if !includeDeleted {
		opts = append(opts, repo.Where("deleted_at", "IS NULL", nil))
	}
	assets, err := s.factory.Assets.List(ctx, opts...)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "asset list: internal error")
	}
	out := make([]gen.Asset, 0, len(assets))
	for _, a := range assets {
		out = append(out, toAsset(a))
	}
	return gen.ListAssets200JSONResponse(out), nil
}

// GetAsset returns a single asset by ID when the requesting user can see it
// under the scope access rule; an asset the user can't see (unknown or another
// user's) fails with 404.
func (s *Server) GetAsset(ctx context.Context, request gen.GetAssetRequestObject) (gen.GetAssetResponseObject, error) {
	asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "asset: internal error")
	}
	if asset == nil {
		return nil, newAPIError(http.StatusNotFound, "asset not found")
	}
	// Soft-deleted assets are hidden unless include_deleted=true.
	if asset.DeletedAt != nil && !(request.Params.IncludeDeleted != nil && *request.Params.IncludeDeleted) {
		return nil, newAPIError(http.StatusNotFound, "asset not found")
	}
	return gen.GetAsset200JSONResponse(toAsset(*asset)), nil
}

// ListAssetDocuments returns all documents attached to one asset. The asset is
// looked up first so that unknown or another user's assets fail with 404
// instead of an empty list.
func (s *Server) ListAssetDocuments(ctx context.Context, request gen.ListAssetDocumentsRequestObject) (gen.ListAssetDocumentsResponseObject, error) {
	if asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "asset: internal error")
	} else if asset == nil {
		return nil, newAPIError(http.StatusNotFound, "asset not found")
	}
	docs, err := s.listDocumentsWithSource(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "document list: internal error")
	}
	out := make([]gen.Document, 0, len(docs))
	for _, d := range docs {
		out = append(out, toDocument(d))
	}
	return gen.ListAssetDocuments200JSONResponse(out), nil
}

// isMaxBytesErr reports whether err (or any error in its chain) is an
// *http.MaxBytesError.
func isMaxBytesErr(err error) bool {
	var mbErr *http.MaxBytesError
	return errors.As(err, &mbErr)
}
