package identity

import (
	"context"
	"errors"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
)

// ErrNoIdentity is returned when the extraction contains no usable
// identity fields (no serial, brand, or model).
var ErrNoIdentity = errors.New("no usable identity fields in extraction")

// Resolve matches an extraction against existing assets using the identity
// hierarchy (serial first, then brand+model), merging into a match or
// creating a new asset. The boolean result reports whether a new asset was
// created. The tenant is extracted from ctx and applied to every repository
// call as a scoping option.
func Resolve(ctx context.Context, assets repo.Repository[entity.Asset], ext entity.Extraction) (entity.Asset, bool, error) {
	tid, err := tenant.TenantFrom(ctx)
	if err != nil {
		return entity.Asset{}, false, err
	}

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
		return resolveBySerial(ctx, assets, tid, ext, normSerial, normBrand, normModel)
	case normBrand != "" && normModel != "":
		return resolveByBrandModel(ctx, assets, tid, ext, normBrand, normModel)
	case normBrand != "" || normModel != "":
		return createNew(ctx, assets, tid, ext, normSerial, normBrand, normModel)
	default:
		return entity.Asset{}, false, ErrNoIdentity
	}
}

// resolveBySerial matches on the normalized serial; on a miss it creates a
// new asset.
func resolveBySerial(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normSerial, normBrand, normModel string) (entity.Asset, bool, error) {
	results, err := assets.List(ctx, repo.Tenant(tid), repo.Where("norm_serial", "=", normSerial), repo.Limit(1))
	if err != nil {
		return entity.Asset{}, false, err
	}
	if len(results) > 0 {
		return mergeInto(ctx, assets, tid, ext, results[0])
	}
	return createNew(ctx, assets, tid, ext, normSerial, normBrand, normModel)
}

// resolveByBrandModel matches on the normalized brand+model pair; on a miss
// it creates a new asset.
func resolveByBrandModel(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normBrand, normModel string) (entity.Asset, bool, error) {
	results, err := assets.List(ctx, repo.Tenant(tid), repo.Where("norm_brand", "=", normBrand), repo.Where("norm_model", "=", normModel), repo.Limit(1))
	if err != nil {
		return entity.Asset{}, false, err
	}
	if len(results) > 0 {
		return mergeInto(ctx, assets, tid, ext, results[0])
	}
	return createNew(ctx, assets, tid, ext, "", normBrand, normModel)
}

// createNew builds a fresh asset from the extraction and stores it.
func createNew(ctx context.Context, assets repo.Repository[entity.Asset], tid string, ext entity.Extraction, normSerial, normBrand, normModel string) (entity.Asset, bool, error) {
	created, err := assets.Create(ctx, newAsset(ext, normSerial, normBrand, normModel), repo.Tenant(tid))
	if err != nil {
		return entity.Asset{}, false, err
	}
	return created, true, nil
}

// newAsset builds a entity.Asset for creation from the extraction. Norm
// columns are nil when the corresponding normalized value is empty.
func newAsset(ext entity.Extraction, normSerial, normBrand, normModel string) entity.Asset {
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
		Brand:        ext.Brand,
		Model:        ext.Model,
		SerialNumber: ext.SerialNumber,
		NormSerial:   normSerialP,
		NormBrand:    normBrandP,
		NormModel:    normModelP,
		PurchaseDate: ext.PurchaseDate,
		WarrantyEnd:  ext.WarrantyEnd,
		Price:        ext.Price,
		Currency:     ext.Currency,
		DocType:      ext.Classification,
		Metadata:     ext.Metadata,
	}
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

	merged, err := assets.Update(ctx, updated, repo.Tenant(tid))
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
