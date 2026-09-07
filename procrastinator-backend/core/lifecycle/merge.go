package lifecycle

import (
	"context"
	"errors"
	"sort"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrInvalidMerge is returned when attempting to merge an asset into itself.
// The API layer maps this sentinel to HTTP 400.
var ErrInvalidMerge = errors.New("cannot merge an asset into itself")

// Merge consolidates the duplicate asset into the survivor asset. The
// survivor's identity (ID, owner, created_at) is preserved; missing (nil)
// scalar fields on the survivor are filled from the duplicate, while any
// non-nil survivor field wins on conflict. Documents linked to the duplicate
// are re-pointed onto the survivor so no document is lost. The duplicate is
// soft-deleted and stamped with merged_into/merged_at as an audit trail. The
// whole operation runs in a single transaction.
func (s *Service) Merge(ctx context.Context, survivorID, duplicateID string) (entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, err
	}
	if survivorID == duplicateID {
		return entity.Asset{}, ErrInvalidMerge
	}

	var survivor entity.Asset
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		sv, err := r.Assets.Get(ctx, survivorID, repo.Owner(tid))
		if err != nil {
			return err
		}
		dup, err := r.Assets.Get(ctx, duplicateID, repo.Owner(tid))
		if err != nil {
			return err
		}
		if sv.DeletedAt != nil {
			return ErrConflict
		}
		now := s.now().UTC()
		merged := mergeIntoAsset(sv, dup)

		if err := repointDocuments(ctx, r, tid, survivorID, duplicateID); err != nil {
			return err
		}

		merged.UpdatedAt = now
		if _, err := r.Assets.Update(ctx, merged, repo.Owner(tid)); err != nil {
			return err
		}

		dup.MergedInto = &survivorID
		dup.MergedAt = &now
		dup.DeletedAt = &now
		dup.UpdatedAt = now
		if _, err := r.Assets.Update(ctx, dup, repo.Owner(tid)); err != nil {
			return err
		}

		survivor = merged
		return nil
	})
	if err != nil {
		return entity.Asset{}, err
	}
	return survivor, nil
}

// MergedAssets returns the assets that have been merged into the given
// survivor. It lists every asset of the bound user (bypassing the soft-delete
// filter, so the soft-deleted duplicates are visible) and filters in Go to the
// ones whose merged_into equals survivorID. The result is sorted by merged_at
// ascending (nil merged_at sorts first).
func (s *Service) MergedAssets(ctx context.Context, survivorID string) ([]entity.Asset, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}

	var out []entity.Asset
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		all, err := r.Assets.List(ctx, repo.Owner(tid))
		if err != nil {
			return err
		}
		for _, a := range all {
			if a.MergedInto != nil && *a.MergedInto == survivorID {
				out = append(out, a)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		mi, mj := out[i].MergedAt, out[j].MergedAt
		if mi == nil && mj == nil {
			return false
		}
		if mi == nil {
			return true
		}
		if mj == nil {
			return false
		}
		return mi.Before(*mj)
	})
	return out, nil
}

// repointDocuments re-points every document currently linked to the duplicate
// asset onto the survivor. The survivor's own documents are left untouched and
// no document row is removed.
func repointDocuments(ctx context.Context, r *repo.Repos, tid, survivorID, duplicateID string) error {
	docs, err := r.Documents.List(ctx, repo.Owner(tid), repo.Where("asset_id", "=", duplicateID))
	if err != nil {
		return err
	}
	for _, d := range docs {
		// Re-point by setting the entity field: the generic Update persists the
		// non-empty asset_id via toMap (no explicit Set needed, unlike detach
		// which must force NULL on an empty field).
		d.AssetID = survivorID
		if _, err := r.Documents.Update(ctx, d, repo.Owner(tid)); err != nil {
			return err
		}
	}
	return nil
}

// mergeIntoAsset is a pure field-strategy merge of the duplicate into the
// survivor. The survivor's identity fields (ID, OwnerID, CreatedAt,
// CategoryUserSet, MergedInto, MergedAt, DeletedAt) are preserved. Nil scalar
// fields on the survivor are filled from the duplicate; non-nil survivor fields
// win. The category is sticky: when the survivor's category is user-set, the
// duplicate's category values are ignored entirely. Metadata is shallow-merged
// per key with the survivor winning on conflict.
func mergeIntoAsset(survivor, duplicate entity.Asset) entity.Asset {
	m := survivor

	m.Brand = coalesce(m.Brand, duplicate.Brand)
	m.Model = coalesce(m.Model, duplicate.Model)
	m.SerialNumber = coalesce(m.SerialNumber, duplicate.SerialNumber)
	m.NormSerial = coalesce(m.NormSerial, duplicate.NormSerial)
	m.NormBrand = coalesce(m.NormBrand, duplicate.NormBrand)
	m.NormModel = coalesce(m.NormModel, duplicate.NormModel)
	m.PurchaseDate = coalesce(m.PurchaseDate, duplicate.PurchaseDate)
	m.WarrantyEnd = coalesce(m.WarrantyEnd, duplicate.WarrantyEnd)
	m.Price = coalesce(m.Price, duplicate.Price)
	m.Currency = coalesce(m.Currency, duplicate.Currency)
	m.Name = coalesce(m.Name, duplicate.Name)
	m.NormName = coalesce(m.NormName, duplicate.NormName)
	m.Confidence = coalesce(m.Confidence, duplicate.Confidence)
	m.OwnerHouseholdID = coalesce(m.OwnerHouseholdID, duplicate.OwnerHouseholdID)

	// Sticky user category: a user-set category on the survivor is never
	// overwritten by the duplicate.
	if !m.CategoryUserSet {
		m.AssetCategory = coalesce(m.AssetCategory, duplicate.AssetCategory)
		m.CategoryConfidence = coalesce(m.CategoryConfidence, duplicate.CategoryConfidence)
	}

	m.Metadata = shallowMergeMetadata(m.Metadata, duplicate.Metadata)
	return m
}

// coalesce returns a when it is non-nil, otherwise b.
func coalesce[T any](a, b *T) *T {
	if a != nil {
		return a
	}
	return b
}

// shallowMergeMetadata merges the two metadata maps per key, with the
// survivor's values winning on conflict. It returns a new map and does not
// mutate either input. A zero-key result is nil.
func shallowMergeMetadata(survivor, dup map[string]any) map[string]any {
	if survivor == nil && dup == nil {
		return nil
	}
	m := make(map[string]any, len(survivor)+len(dup))
	for k, v := range dup {
		m[k] = v
	}
	for k, v := range survivor {
		m[k] = v
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
