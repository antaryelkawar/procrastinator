package api

import (
	"time"

	"procrastinator-backend/commons/entity"
)

// assetJSON is the JSON representation of a entity.Asset returned by the API.
type assetJSON struct {
	ID           string         `json:"id"`
	Brand        *string        `json:"brand,omitempty"`
	Model        *string        `json:"model,omitempty"`
	SerialNumber *string        `json:"serial_number,omitempty"`
	PurchaseDate *time.Time     `json:"purchase_date,omitempty"`
	WarrantyEnd  *time.Time     `json:"warranty_end,omitempty"`
	Price        *string        `json:"price,omitempty"`
	Currency     *string        `json:"currency,omitempty"`
	DocType      string         `json:"doc_type"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	// OwnerHouseholdID is the owner_household_id column; omitted (null) for
	// personal rows.
	OwnerHouseholdID *string `json:"owner_household_id,omitempty"`
}

// documentJSON is the JSON representation of a entity.DocumentWithSource
// returned by the API.
type documentJSON struct {
	ID               string    `json:"id"`
	DocType          string    `json:"doc_type"`
	SourceFilename   string    `json:"source_filename"`
	SourceUploadedAt time.Time `json:"source_uploaded_at"`
	CreatedAt        time.Time `json:"created_at"`
	// OwnerHouseholdID is the document's owner_household_id column; omitted
	// (null) for personal rows.
	OwnerHouseholdID *string `json:"owner_household_id,omitempty"`
}

// toAssetJSON converts a entity.Asset into its JSON DTO. Metadata is
// initialized to an empty map so it marshals as {} rather than null.
func toAssetJSON(a entity.Asset) assetJSON {
	metadata := a.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return assetJSON{
		ID:               a.ID,
		Brand:            a.Brand,
		Model:            a.Model,
		SerialNumber:     a.SerialNumber,
		PurchaseDate:     a.PurchaseDate,
		WarrantyEnd:      a.WarrantyEnd,
		Price:            a.Price,
		Currency:         a.Currency,
		DocType:          a.DocType,
		Metadata:         metadata,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
		OwnerHouseholdID: a.OwnerHouseholdID,
	}
}

// toDocumentJSON converts a entity.DocumentWithSource into its JSON DTO.
func toDocumentJSON(d entity.DocumentWithSource) documentJSON {
	return documentJSON{
		ID:               d.ID,
		DocType:          d.DocType,
		SourceFilename:   d.SourceFilename,
		SourceUploadedAt: d.SourceUploadedAt,
		CreatedAt:        d.CreatedAt,
		OwnerHouseholdID: d.OwnerHouseholdID,
	}
}
