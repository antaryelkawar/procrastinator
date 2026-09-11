package assets

import (
	"context"
	"net/http"

	"procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/lifecycle"
)

// Service serves the asset read endpoints (list, get, documents).
type Service struct {
	factory   *repo.Factory
	lifecycle *lifecycle.Service
}

// New constructs a Service.
func New(factory *repo.Factory, lifecycle *lifecycle.Service) *Service {
	return &Service{factory: factory, lifecycle: lifecycle}
}

// findAssetScoped returns the single asset with id visible to the user under
// the scope access rule, or nil when the user cannot see it (unknown or
// another user's assets both surface as not-found).
func (s *Service) findAssetScoped(ctx context.Context, tid, id string) (*entity.Asset, error) {
	return dto.FindAssetScoped(s.factory, ctx, tid, id)
}

// listDocumentsWithSource returns the documents attached to assetID visible to
// the user under the scope access rule, joined with their source metadata,
// ordered by created_at then id.
func (s *Service) listDocumentsWithSource(ctx context.Context, tid, assetID string) ([]entity.DocumentWithSource, error) {
	list, err := s.factory.Documents.List(ctx, repo.Owner(tid), repo.Where("asset_id", "=", assetID), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return []entity.DocumentWithSource{}, nil
	}
	out := make([]entity.DocumentWithSource, 0, len(list))
	for _, d := range list {
		// Fetch the source through the same scope access rule as the document.
		// A household document's source is owned by the same household, so a
		// member must be able to read it even though its owner_id is the
		// owning user rather than the requesting member (the generic
		// owner-scoped Get would hide it from members).
		srcs, err := s.factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", d.SourceID))
		if err != nil {
			return nil, err
		}
		if len(srcs) == 0 {
			// Unreachable when the document is visible (the source shares its
			// scope); fail closed rather than emit a partial row.
			return nil, repo.ErrNotFound
		}
		dw := entity.DocumentWithSource{Document: d}
		dw.SourceFilename = srcs[0].Filename
		dw.SourceUploadedAt = srcs[0].UploadedAt
		out = append(out, dw)
	}
	return out, nil
}

// ListAssets returns all assets visible to the requesting user under the
// scope access rule.
func (s *Service) ListAssets(ctx context.Context, request gen.ListAssetsRequestObject) (gen.ListAssetsResponseObject, error) {
	opts := []repo.Option{repo.Owner(request.UserId)}
	includeDeleted := request.Params.IncludeDeleted != nil && *request.Params.IncludeDeleted
	if !includeDeleted {
		opts = append(opts, repo.Where("deleted_at", "IS NULL", nil))
	}
	assets, err := s.factory.Assets.List(ctx, opts...)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset list: internal error")
	}
	out := make([]gen.Asset, 0, len(assets))
	for _, a := range assets {
		out = append(out, dto.ToAsset(a))
	}
	return gen.ListAssets200JSONResponse(out), nil
}

// GetAsset returns a single asset by ID when the requesting user can see it
// under the scope access rule; an asset the user can't see (unknown or another
// user's) fails with 404.
func (s *Service) GetAsset(ctx context.Context, request gen.GetAssetRequestObject) (gen.GetAssetResponseObject, error) {
	asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	}
	if asset == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	// Soft-deleted assets are hidden unless include_deleted=true.
	if asset.DeletedAt != nil && !(request.Params.IncludeDeleted != nil && *request.Params.IncludeDeleted) {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	return gen.GetAsset200JSONResponse(dto.ToAsset(*asset)), nil
}

// ListAssetDocuments returns all documents attached to one asset. The asset is
// looked up first so that unknown or another user's assets fail with 404
// instead of an empty list.
func (s *Service) ListAssetDocuments(ctx context.Context, request gen.ListAssetDocumentsRequestObject) (gen.ListAssetDocumentsResponseObject, error) {
	if asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId); err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	} else if asset == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	docs, err := s.listDocumentsWithSource(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "document list: internal error")
	}
	out := make([]gen.Document, 0, len(docs))
	for _, d := range docs {
		// Documents attached to a live asset are, by construction, processed
		// (they resolved to this asset at commit/approval time).
		out = append(out, dto.ToDocument(d, gen.DocumentStatusProcessed))
	}
	return gen.ListAssetDocuments200JSONResponse(out), nil
}
