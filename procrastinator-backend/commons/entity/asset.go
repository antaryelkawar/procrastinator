package entity

import "time"

// Asset category vocabulary (intrinsic to the asset, not the document).
const (
	AssetCategoryAppliance    = "appliance"
	AssetCategoryElectronics  = "electronics"
	AssetCategoryComputing    = "computing"
	AssetCategoryFurniture    = "furniture"
	AssetCategoryVehicle      = "vehicle"
	AssetCategoryTool         = "tool"
	AssetCategoryClothing     = "clothing"
	AssetCategoryDocumentOnly = "document_only"
	AssetCategoryOther        = "other"
)

// ValidAssetCategory returns true if s is exactly one of the asset category
// vocabulary constants (case-sensitive).
func ValidAssetCategory(s string) bool {
	switch s {
	case AssetCategoryAppliance, AssetCategoryElectronics, AssetCategoryComputing,
		AssetCategoryFurniture, AssetCategoryVehicle, AssetCategoryTool,
		AssetCategoryClothing, AssetCategoryDocumentOnly, AssetCategoryOther:
		return true
	}
	return false
}

// Asset represents an owner-scoped device record with structured core fields
// and open metadata.
type Asset struct {
	ID               string
	OwnerID          string
	Brand            *string
	Model            *string
	SerialNumber     *string
	NormSerial       *string
	NormBrand        *string
	NormModel        *string
	PurchaseDate     *time.Time
	WarrantyEnd      *time.Time
	Price            *string
	Currency         *string
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
	// Confidence is the LLM extraction confidence (0.0–1.0) stamped at
	// create/merge time; nil means absent.
	Confidence *float64
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
	// Name is the canonical product name (e.g. "Microwave Oven"); nil means absent.
	Name *string
	// NormName is the normalized form of Name (lowercase, trimmed); nil means absent.
	NormName *string
	// AssetCategory is the intrinsic asset category (one of the AssetCategory*
	// constants); nil means unclassified.
	AssetCategory *string
	// CategoryConfidence is the inference confidence (0.0–1.0) for
	// AssetCategory; nil means absent.
	CategoryConfidence *float64
	// CategoryUserSet is true when the user has explicitly set the category;
	// the extractor must not overwrite a user-set category.
	CategoryUserSet bool
	// DeletedAt is the soft-delete timestamp; nil means the asset is active.
	DeletedAt *time.Time
	// MergedInto is the ID of the surviving asset after a merge; nil means not merged.
	MergedInto *string
	// MergedAt is when the merge occurred; nil means not merged.
	MergedAt *time.Time
}
