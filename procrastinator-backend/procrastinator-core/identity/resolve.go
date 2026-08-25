package identity

import (
	"context"
	"errors"

	"procrastinator-backend/commons-data"
)

// ErrNoIdentity is returned when the extraction contains no usable
// identity fields (no serial, brand, or model).
var ErrNoIdentity = errors.New("no usable identity fields in extraction")

// Resolve matches an extraction against existing assets using the identity
// hierarchy (serial first, then brand+model), merging into a match or
// creating a new asset. The boolean result reports whether a new asset was
// created.
func Resolve(ctx context.Context, repo data.AssetRepository, ext data.Extraction) (data.Asset, bool, error) {
	var normSerial, normBrand, normModel string
	if ext.SerialNumber != nil {
		normSerial = data.NormalizeSerial(*ext.SerialNumber)
	}
	if ext.Brand != nil {
		normBrand = data.NormalizeName(*ext.Brand)
	}
	if ext.Model != nil {
		normModel = data.NormalizeName(*ext.Model)
	}

	switch {
	case normSerial != "":
		return resolveBySerial(ctx, repo, ext, normSerial, normBrand, normModel)
	case normBrand != "" && normModel != "":
		return resolveByBrandModel(ctx, repo, ext, normBrand, normModel)
	case normBrand != "" || normModel != "":
		return createNew(ctx, repo, ext, normSerial, normBrand, normModel)
	default:
		return data.Asset{}, false, ErrNoIdentity
	}
}

// resolveBySerial matches on the normalized serial; on a miss it creates
// with serial-conflict handling, merging into the winner on conflict.
func resolveBySerial(ctx context.Context, repo data.AssetRepository, ext data.Extraction, normSerial, normBrand, normModel string) (data.Asset, bool, error) {
	found, err := repo.FindBySerial(ctx, normSerial)
	if err == nil {
		return mergeInto(ctx, repo, ext, found)
	}
	if !errors.Is(err, data.ErrNotFound) {
		return data.Asset{}, false, err
	}

	created, conflict, err := repo.CreateOnConflictSerial(ctx, newAsset(ext, normSerial, normBrand, normModel))
	if err != nil {
		return data.Asset{}, false, err
	}
	if conflict {
		return mergeInto(ctx, repo, ext, created)
	}
	return created, true, nil
}

// resolveByBrandModel matches on the normalized brand+model pair; on a miss
// it creates a new asset.
func resolveByBrandModel(ctx context.Context, repo data.AssetRepository, ext data.Extraction, normBrand, normModel string) (data.Asset, bool, error) {
	found, err := repo.FindByBrandModel(ctx, normBrand, normModel)
	if err == nil {
		return mergeInto(ctx, repo, ext, found)
	}
	if !errors.Is(err, data.ErrNotFound) {
		return data.Asset{}, false, err
	}
	return createNew(ctx, repo, ext, "", normBrand, normModel)
}

// createNew builds a fresh asset from the extraction and stores it.
func createNew(ctx context.Context, repo data.AssetRepository, ext data.Extraction, normSerial, normBrand, normModel string) (data.Asset, bool, error) {
	created, err := repo.Create(ctx, newAsset(ext, normSerial, normBrand, normModel))
	if err != nil {
		return data.Asset{}, false, err
	}
	return created, true, nil
}

// newAsset builds a data.Asset for creation from the extraction. Norm
// columns are nil when the corresponding normalized value is empty.
func newAsset(ext data.Extraction, normSerial, normBrand, normModel string) data.Asset {
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
	return data.Asset{
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

// mergeInto applies the non-nil extraction fields onto an existing asset.
func mergeInto(ctx context.Context, repo data.AssetRepository, ext data.Extraction, asset data.Asset) (data.Asset, bool, error) {
	updated, err := repo.UpdateFields(ctx, asset.ID, buildUpdateFields(ext, asset.Metadata))
	if err != nil {
		return data.Asset{}, false, err
	}
	return updated, false, nil
}

// buildUpdateFields maps non-nil extraction fields onto an UpdateFields
// struct. DocType is always set when the classification is non-empty
// (last write wins); Metadata is the shallow merge of existing and incoming.
func buildUpdateFields(ext data.Extraction, existingMetadata map[string]any) data.UpdateFields {
	f := data.UpdateFields{
		Brand:        ext.Brand,
		Model:        ext.Model,
		SerialNumber: ext.SerialNumber,
		PurchaseDate: ext.PurchaseDate,
		WarrantyEnd:  ext.WarrantyEnd,
		Price:        ext.Price,
		Currency:     ext.Currency,
	}
	if ext.Classification != "" {
		f.DocType = &ext.Classification
	}
	f.Metadata = mergeMetadata(existingMetadata, ext.Metadata)
	return f
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
