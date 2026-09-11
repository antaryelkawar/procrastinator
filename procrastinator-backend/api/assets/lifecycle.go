package assets

import (
	"context"
	"errors"
	"net/http"
	"time"

	"procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/lifecycle"
)

// DeleteAsset soft-deletes an asset. The asset is verified against the scope
// access rule first (unknown or another user's asset → 404) before the
// lifecycle service stamps deleted_at.
func (s *Service) DeleteAsset(ctx context.Context, request gen.DeleteAssetRequestObject) (gen.DeleteAssetResponseObject, error) {
	asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	}
	if asset == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	if _, err := s.lifecycle.SoftDelete(ctx, request.AssetId); err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset delete: internal error")
	}
	return gen.DeleteAsset204Response{}, nil
}

// RestoreAsset clears the soft-delete on an asset. An asset beyond the retention
// window returns 409.
func (s *Service) RestoreAsset(ctx context.Context, request gen.RestoreAssetRequestObject) (gen.RestoreAssetResponseObject, error) {
	asset, err := s.findAssetScoped(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	}
	if asset == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	restored, err := s.lifecycle.Restore(ctx, request.AssetId)
	if errors.Is(err, lifecycle.ErrConflict) {
		return nil, httpx.NewAPIError(http.StatusConflict, "asset is beyond the retention window and can no longer be restored")
	}
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset restore: internal error")
	}
	return gen.RestoreAsset200JSONResponse(dto.ToAsset(restored)), nil
}

// MergeAsset merges a duplicate asset into the survivor (the path assetId).
// A self-merge or a missing survivor/duplicate is rejected; the response carries
// the survivor with its merged-assets list populated.
func (s *Service) MergeAsset(ctx context.Context, request gen.MergeAssetRequestObject) (gen.MergeAssetResponseObject, error) {
	if request.Body == nil {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "missing request body")
	}
	survivorID := request.AssetId
	duplicateID := request.Body.DuplicateAssetId
	if survivorID == duplicateID {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "cannot merge an asset into itself")
	}
	if sv, err := s.findAssetScoped(ctx, request.UserId, survivorID); err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	} else if sv == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}
	if dup, err := s.findAssetScoped(ctx, request.UserId, duplicateID); err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	} else if dup == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}

	survivor, err := s.lifecycle.Merge(ctx, survivorID, duplicateID)
	if errors.Is(err, lifecycle.ErrInvalidMerge) {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "cannot merge an asset into itself")
	}
	if errors.Is(err, lifecycle.ErrConflict) {
		return nil, httpx.NewAPIError(http.StatusConflict, "conflicting state")
	}
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset merge: internal error")
	}

	assetDTO := dto.ToAsset(survivor)

	// Populate the survivor's merged-assets list; a fetch failure is not fatal
	// to the response (the merge already committed).
	if merged, err := s.lifecycle.MergedAssets(ctx, survivorID); err == nil && len(merged) > 0 {
		list := make([]struct {
			AssetId  string    `json:"asset_id"`
			MergedAt time.Time `json:"merged_at"`
		}, 0, len(merged))
		for _, m := range merged {
			ma := m.MergedAt
			if ma == nil {
				zero := time.Time{}
				ma = &zero
			}
			list = append(list, struct {
				AssetId  string    `json:"asset_id"`
				MergedAt time.Time `json:"merged_at"`
			}{AssetId: m.ID, MergedAt: *ma})
		}
		assetDTO.Data.MergedAssets = &list
	}

	return gen.MergeAsset200JSONResponse(assetDTO), nil
}

// PatchAsset applies a partial update to an asset. Only the provided (non-nil)
// fields are applied; setting asset_category marks it as user-set (sticky).
func (s *Service) PatchAsset(ctx context.Context, request gen.PatchAssetRequestObject) (gen.PatchAssetResponseObject, error) {
	if request.Body == nil {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "missing request body")
	}
	found, err := s.findAssetScoped(ctx, request.UserId, request.AssetId)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset: internal error")
	}
	if found == nil {
		return nil, httpx.NewAPIError(http.StatusNotFound, "asset not found")
	}

	asset := *found
	if request.Body.Brand != nil {
		asset.Brand = request.Body.Brand
	}
	if request.Body.Model != nil {
		asset.Model = request.Body.Model
	}
	if request.Body.SerialNumber != nil {
		asset.SerialNumber = request.Body.SerialNumber
	}
	if request.Body.Name != nil {
		asset.Name = request.Body.Name
	}
	if request.Body.Price != nil {
		asset.Price = request.Body.Price
	}
	if request.Body.Currency != nil {
		asset.Currency = request.Body.Currency
	}
	if request.Body.AssetCategory != nil {
		cat := string(*request.Body.AssetCategory)
		asset.AssetCategory = &cat
		asset.CategoryUserSet = true
	}

	updated, err := s.factory.Assets.Update(ctx, asset, repo.Owner(request.UserId))
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "asset patch: internal error")
	}
	return gen.PatchAsset200JSONResponse(dto.ToAsset(updated)), nil
}
