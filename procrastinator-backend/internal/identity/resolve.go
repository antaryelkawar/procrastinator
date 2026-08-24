// resolve.go provides identity resolution: matching an LLM extraction to
// an existing asset or creating a new one, with field merge on match.
package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"procrastinator-backend/internal/llm/extraction"
)

// ErrNoIdentity is returned by Resolve when the extraction contains no
// usable identity fields (no serial number and no brand+model pair).
var ErrNoIdentity = errors.New("identity: no usable identity fields")

// Asset represents a physical or virtual asset identified by its serial
// number, brand, and model, with optional purchase and warranty metadata.
type Asset struct {
	// ID is the unique identifier assigned by the store.
	ID string
	// Brand is the product brand. Nil if unknown.
	Brand *string
	// Model is the product model. Nil if unknown.
	Model *string
	// SerialNumber is the product serial number. Nil if unknown.
	SerialNumber *string
	// PurchaseDate is the date of purchase. Nil if unknown.
	PurchaseDate *time.Time
	// Price is the purchase price as a decimal string. Nil if unknown.
	Price *string
	// Currency is the ISO 4217 currency code. Nil if unknown.
	Currency *string
	// WarrantyStart is the warranty start date. Nil if unknown.
	WarrantyStart *time.Time
	// WarrantyEnd is the warranty end date. Nil if unknown.
	WarrantyEnd *time.Time
	// CreatedAt is the timestamp when the asset was created.
	CreatedAt time.Time
	// UpdatedAt is the timestamp when the asset was last updated.
	UpdatedAt time.Time
}

// UpdateFields specifies which fields of an Asset to update. Nil fields
// are left untouched; non-nil fields are applied.
type UpdateFields struct {
	// Brand is the new brand value. Nil to leave unchanged.
	Brand *string
	// Model is the new model value. Nil to leave unchanged.
	Model *string
	// SerialNumber is the new serial number. Nil to leave unchanged.
	SerialNumber *string
	// PurchaseDate is the new purchase date. Nil to leave unchanged.
	PurchaseDate *time.Time
	// Price is the new price. Nil to leave unchanged.
	Price *string
	// Currency is the new currency. Nil to leave unchanged.
	Currency *string
	// WarrantyStart is the new warranty start date. Nil to leave unchanged.
	WarrantyStart *time.Time
	// WarrantyEnd is the new warranty end date. Nil to leave unchanged.
	WarrantyEnd *time.Time
}

// AssetStore is the narrow persistence surface for asset identity
// resolution. Implementations honor any transaction bound to ctx; Resolve
// never starts one.
type AssetStore interface {
	// FindBySerial looks up an asset by its normalized serial number.
	FindBySerial(ctx context.Context, normSerial string) (Asset, bool, error)
	// FindByBrandModel looks up an asset by normalized brand and model.
	FindByBrandModel(ctx context.Context, normBrand, normModel string) (Asset, bool, error)
	// Create inserts a new asset and returns it with store-assigned fields.
	Create(ctx context.Context, a Asset) (Asset, error)
	// CreateOnConflictSerial inserts a new asset atomically; on serial
	// conflict it returns the existing row with created=false.
	CreateOnConflictSerial(ctx context.Context, a Asset) (Asset, bool, error)
	// UpdateFields applies non-nil fields to the asset identified by id.
	UpdateFields(ctx context.Context, id string, f UpdateFields) (Asset, error)
}

// Resolve matches an extraction to an existing asset or creates a new one.
//
// Resolution precedence:
//  1. If the extraction has a serial number, look up by serial. On match,
//     merge extraction fields into the existing asset. On miss, race-safe
//     insert; on conflict, merge into the existing row.
//  2. Otherwise, if both brand and model are present, look up by
//     brand+model. On match, merge. On miss, plain insert.
//  3. Otherwise, return ErrNoIdentity with no store interaction.
//
// The returned bool indicates whether a new asset was created.
func Resolve(ctx context.Context, st AssetStore, ex extraction.Extraction) (Asset, bool, error) {
	ns := NormalizeSerial(ex.SerialNumber)
	nb := NormalizeName(ex.Brand)
	nm := NormalizeName(ex.Model)

	hasSerial := ns != ""
	hasBrandModel := nb != "" && nm != ""

	if !hasSerial && !hasBrandModel {
		return Asset{}, false, ErrNoIdentity
	}

	if hasSerial {
		// Serial takes precedence over brand+model.
		found, ok, err := st.FindBySerial(ctx, ns)
		if err != nil {
			return Asset{}, false, fmt.Errorf("identity: find by serial: %w", err)
		}
		if ok {
			merged, err := merge(ctx, st, found, ex)
			if err != nil {
				return Asset{}, false, err
			}
			return merged, false, nil
		}
		// Not found — race-safe create.
		a := assetFromExtraction(ex)
		created, wasCreated, err := st.CreateOnConflictSerial(ctx, a)
		if err != nil {
			return Asset{}, false, fmt.Errorf("identity: create on conflict serial: %w", err)
		}
		if wasCreated {
			return created, true, nil
		}
		// Conflict — the store returned the existing row; merge into it.
		merged, err := merge(ctx, st, created, ex)
		if err != nil {
			return Asset{}, false, err
		}
		return merged, false, nil
	}

	// Brand+model path (no serial).
	found, ok, err := st.FindByBrandModel(ctx, nb, nm)
	if err != nil {
		return Asset{}, false, fmt.Errorf("identity: find by brand/model: %w", err)
	}
	if ok {
		merged, err := merge(ctx, st, found, ex)
		if err != nil {
			return Asset{}, false, err
		}
		return merged, false, nil
	}
	// Not found — plain create.
	a := assetFromExtraction(ex)
	created, err := st.Create(ctx, a)
	if err != nil {
		return Asset{}, false, fmt.Errorf("identity: create: %w", err)
	}
	return created, true, nil
}

// assetFromExtraction builds an Asset from an extraction. Display values
// for Brand, Model, and SerialNumber are whitespace-collapsed but
// case-preserved. Only non-empty / non-zero fields are populated; absent
// fields remain nil.
func assetFromExtraction(ex extraction.Extraction) Asset {
	var a Asset
	if s := collapseWhitespace(ex.Brand); s != "" {
		a.Brand = &s
	}
	if s := collapseWhitespace(ex.Model); s != "" {
		a.Model = &s
	}
	if s := collapseWhitespace(ex.SerialNumber); s != "" {
		a.SerialNumber = &s
	}
	if !ex.PurchaseDate.IsZero() {
		t := ex.PurchaseDate
		a.PurchaseDate = &t
	}
	if ex.Price != "" {
		s := ex.Price
		a.Price = &s
	}
	if ex.Currency != "" {
		s := ex.Currency
		a.Currency = &s
	}
	if !ex.WarrantyStart.IsZero() {
		t := ex.WarrantyStart
		a.WarrantyStart = &t
	}
	if !ex.WarrantyEnd.IsZero() {
		t := ex.WarrantyEnd
		a.WarrantyEnd = &t
	}
	return a
}

// buildUpdateFields constructs an UpdateFields from an extraction. Only
// fields that are present (non-empty after whitespace collapse for strings,
// non-zero for times) are set to non-nil pointers; absent fields remain
// nil to signal "leave untouched" to UpdateFields.
func buildUpdateFields(ex extraction.Extraction) UpdateFields {
	var f UpdateFields
	if s := collapseWhitespace(ex.Brand); s != "" {
		f.Brand = &s
	}
	if s := collapseWhitespace(ex.Model); s != "" {
		f.Model = &s
	}
	if s := collapseWhitespace(ex.SerialNumber); s != "" {
		f.SerialNumber = &s
	}
	if !ex.PurchaseDate.IsZero() {
		t := ex.PurchaseDate
		f.PurchaseDate = &t
	}
	if ex.Price != "" {
		s := ex.Price
		f.Price = &s
	}
	if ex.Currency != "" {
		s := ex.Currency
		f.Currency = &s
	}
	if !ex.WarrantyStart.IsZero() {
		t := ex.WarrantyStart
		f.WarrantyStart = &t
	}
	if !ex.WarrantyEnd.IsZero() {
		t := ex.WarrantyEnd
		f.WarrantyEnd = &t
	}
	return f
}

// merge applies the extraction's present fields to an existing asset via
// UpdateFields. If no fields are present (defensive), the matched asset
// is returned unchanged with no store call.
func merge(ctx context.Context, st AssetStore, a Asset, ex extraction.Extraction) (Asset, error) {
	f := buildUpdateFields(ex)
	if !hasAnyField(f) {
		return a, nil
	}
	updated, err := st.UpdateFields(ctx, a.ID, f)
	if err != nil {
		return Asset{}, fmt.Errorf("identity: update fields: %w", err)
	}
	return updated, nil
}

// hasAnyField reports whether at least one field in f is non-nil.
func hasAnyField(f UpdateFields) bool {
	return f.Brand != nil ||
		f.Model != nil ||
		f.SerialNumber != nil ||
		f.PurchaseDate != nil ||
		f.Price != nil ||
		f.Currency != nil ||
		f.WarrantyStart != nil ||
		f.WarrantyEnd != nil
}
