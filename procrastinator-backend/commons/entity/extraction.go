package entity

import "time"

// Extraction represents validated fields extracted from an LLM response.
// Nil pointer fields mean absent. Warranty start dates are NOT in the core—they live in Metadata.
type Extraction struct {
	Classification string
	Brand          *string
	Model          *string
	SerialNumber   *string
	PurchaseDate   *time.Time
	WarrantyEnd    *time.Time
	Price          *string
	Currency       *string
	Metadata       map[string]any
	RawPayload     string
	// Confidence is the LLM's self-assessed confidence in the extraction
	// (0.0–1.0); nil means absent (the LLM did not emit it or it was
	// unparseable/out-of-range).
	Confidence *float64
	// Name is the asset name; nil means absent.
	Name *string
	// WarrantyDuration is the warranty duration string; empty means absent.
	WarrantyDuration string
	// AssetCategory is the asset category; nil means absent.
	AssetCategory *string
	// CategoryConfidence is the inference confidence (0.0–1.0) for
	// AssetCategory; nil means absent. Stamped onto the Asset at
	// create/merge time (identity resolution).
	CategoryConfidence *float64
}
