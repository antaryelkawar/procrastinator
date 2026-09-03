package api

import (
	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
)

// toAsset converts a entity.Asset into the generated gen.Asset DTO. Metadata is
// initialized to an empty map so it marshals as {} rather than null.
func toAsset(a entity.Asset) gen.Asset {
	metadata := a.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return gen.Asset{
		Id:               a.ID,
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
		OwnerHouseholdId: a.OwnerHouseholdID,
	}
}

// toDocument converts a entity.DocumentWithSource into the generated gen.Document DTO.
func toDocument(d entity.DocumentWithSource) gen.Document {
	return gen.Document{
		Id:               d.ID,
		DocType:          d.DocType,
		SourceFilename:   d.SourceFilename,
		SourceUploadedAt: d.SourceUploadedAt,
		CreatedAt:        d.CreatedAt,
		OwnerHouseholdId: d.OwnerHouseholdID,
	}
}
