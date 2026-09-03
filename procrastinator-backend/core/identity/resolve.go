package identity

import (
	"context"
	"errors"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrNoIdentity is returned when the extraction contains no usable
// identity fields (no serial, brand, or model).
var ErrNoIdentity = errors.New("no usable identity fields in extraction")

// Resolve matches an extraction against existing assets using the identity
// hierarchy (serial first, then brand+model), merging into a match or
// creating a new asset. The boolean result reports whether a new asset was
// created. The owner is extracted from ctx and applied to every repository
// call via repo.Owner(tid). The ownerHouseholdID parameter fences resolution
// to a scope: a non-nil value restricts matches to the given household, a nil
// value restricts them to personal (non-household) assets, so a household
// upload never merges into a personal asset (and vice versa). A newly-created
// asset is stamped with the same scope.
func Resolve(ctx context.Context, assets repo.Repository[entity.Asset], ext entity.Extraction, ownerHouseholdID *string) (entity.Asset, bool, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Asset{}, false, err
	}

	fence := scopeFence(ownerHouseholdID)

	var normSerial, normBrand, normModel string
	if ext.SerialNumber != nil {
		normSerial = commons.NormalizeSerial(*ext.SerialNumber)
	}
	if ext.Brand != nil {
		normBrand = commons.NormalizeName(*ext.Brand)
	}
	if ext.Model != nil {
		normModel = commons.NormalizeName(*ext.Model)
	}

	switch {
	case normSerial != "":
		return resolveBySerial(ctx, assets, tid, ext, normSerial, normBrand, normModel, fence)
	case normBrand != "" && normModel != "":
		return resolveByBrandModel(ctx, assets, tid, ext, normBrand, normModel, fence)
	case normBrand != "" || normModel != "":
		return createNew(ctx, assets, tid, ext, normSerial, normBrand, normModel, fence)
	default:
		return entity.Asset{}, false, ErrNoIdentity
	}
}

// scopeFence builds the scope-filtering options for ownerHouseholdID: a
// non-nil value restricts to that household, nil restricts to personal
// (owner_household_id IS NULL) assets.
func scopeFence(ownerHouseholdID *string) []repo.Option {
	if ownerHouseholdID != nil {
		return []repo.Option{repo.Where("owner_household_id", "=", *ownerHouseholdID)}
	}
	return []repo.Option{repo.Where("owner_household_id", "IS NULL", nil)}
}

// resolveBySerial matches on the normalized serial; on a miss it creates a
// new asset.
func resolveBySerial(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normSerial, normBrand, normModel string, fence []repo.Option) (entity.Asset, bool, error) {
	opts := append([]repo.Option{repo.Owner(tid), repo.Where("norm_serial", "=", normSerial)}, fence...)
	opts = append(opts, repo.Limit(1))
	results, err := assets.List(ctx, opts...)
	if err != nil {
		return entity.Asset{}, false, err
	}
	if len(results) > 0 {
		return mergeInto(ctx, assets, tid, ext, results[0])
	}
	return createNew(ctx, assets, tid, ext, normSerial, normBrand, normModel, fence)
}

// resolveByBrandModel matches on the normalized brand+model pair; on a miss
// it creates a new asset.
func resolveByBrandModel(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normBrand, normModel string, fence []repo.Option) (entity.Asset, bool, error) {
	opts := append([]repo.Option{repo.Owner(tid), repo.Where("norm_brand", "=", normBrand), repo.Where("norm_model", "=", normModel)}, fence...)
	opts = append(opts, repo.Limit(1))
	results, err := assets.List(ctx, opts...)
	if err != nil {
		return entity.Asset{}, false, err
	}
	if len(results) > 0 {
		return mergeInto(ctx, assets, tid, ext, results[0])
	}
	return createNew(ctx, assets, tid, ext, "", normBrand, normModel, fence)
}

// createNew builds a fresh asset from the extraction and stores it.
func createNew(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normSerial, normBrand, normModel string, fence []repo.Option) (entity.Asset, bool, error) {
	created, err := assets.Create(ctx, newAsset(ext, normSerial, normBrand, normModel, fence), repo.Owner(tid))
	if err != nil {
		return entity.Asset{}, false, err
	}
	return created, true, nil
}

// newAsset builds a entity.Asset for creation from the extraction. Norm
// columns are nil when the corresponding normalized value is empty. The
// owner_household_id scope is stamped from the fence (nil for a personal
// asset).
func newAsset(ext entity.Extraction, normSerial, normBrand, normModel string, fence []repo.Option) entity.Asset {
	var normSerialP, normBrandP, normModelP *string
	if normSerial != "" {
		normSerialP = &normSerial
	}
	if normBrand != "" {
		normBrandP = &normBrand
	}
	if normModel != "" {
		normModelP = &normModel
	}
	return entity.Asset{
		Brand:            ext.Brand,
		Model:            ext.Model,
		SerialNumber:     ext.SerialNumber,
		NormSerial:       normSerialP,
		NormBrand:        normBrandP,
		NormModel:        normModelP,
		PurchaseDate:     ext.PurchaseDate,
		WarrantyEnd:      ext.WarrantyEnd,
		Price:            ext.Price,
		Currency:         ext.Currency,
		DocType:          ext.Classification,
		Metadata:         ext.Metadata,
		OwnerHouseholdID: scopeFromFence(fence),
	}
}

// scopeFromFence recovers the owner_household_id scope from the fence options.
// A fence carrying "IS NULL" yields nil (personal); a fence carrying "=" on a
// household ID yields that household ID. An empty fence yields nil.
func scopeFromFence(fence []repo.Option) *string {
	for _, opt := range fence {
		o := repo.ApplyOptions(opt)
		for _, f := range o.Filters {
			if f.Field != "owner_household_id" {
				continue
			}
			if f.Op == "IS NULL" {
				return nil
			}
			if f.Op == "=" {
				if v, ok := f.Value.(string); ok {
					return &v
				}
			}
		}
	}
	return nil
}

// mergeInto applies the non-nil extraction fields onto a copy of the existing
// asset and persists the full entity.
func mergeInto(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, existing entity.Asset) (entity.Asset, bool, error) {
	updated := existing
	if ext.Brand != nil {
		updated.Brand = ext.Brand
	}
	if ext.Model != nil {
		updated.Model = ext.Model
	}
	if ext.SerialNumber != nil {
		updated.SerialNumber = ext.SerialNumber
	}
	if ext.PurchaseDate != nil {
		updated.PurchaseDate = ext.PurchaseDate
	}
	if ext.WarrantyEnd != nil {
		updated.WarrantyEnd = ext.WarrantyEnd
	}
	if ext.Price != nil {
		updated.Price = ext.Price
	}
	if ext.Currency != nil {
		updated.Currency = ext.Currency
	}
	if ext.Classification != "" {
		updated.DocType = ext.Classification
	}
	if ext.Metadata != nil {
		updated.Metadata = mergeMetadata(existing.Metadata, ext.Metadata)
	}

	merged, err := assets.Update(ctx, updated, repo.Owner(tid))
	if err != nil {
		return entity.Asset{}, false, err
	}
	return merged, false, nil
}

// mergeMetadata shallow-merges incoming metadata over existing metadata.
// A nil incoming map yields nil (no metadata update).
func mergeMetadata(existing, incoming map[string]any) map[string]any {
	if incoming == nil {
		return nil
	}
	merged := make(map[string]any, len(existing)+len(incoming))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range incoming {
		merged[k] = v
	}
	return merged
}
