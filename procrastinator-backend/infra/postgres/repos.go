package postgres

import (
	"encoding/json"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// NewAssetRepository returns a generic repository for entity.Asset.
func NewAssetRepository(q Querier) repo.Repository[entity.Asset] {
	return &pgRepository[entity.Asset]{
		q:       q,
		table:   "assets",
		scanRow: scanAsset,
		toMap:   assetToMap,
	}
}

// NewSourceRepository returns a generic repository for entity.Source.
func NewSourceRepository(q Querier) repo.Repository[entity.Source] {
	return &pgRepository[entity.Source]{
		q:       q,
		table:   "sources",
		scanRow: scanSource,
		toMap:   sourceToMap,
	}
}

// NewDocumentRepository returns a generic repository for entity.Document.
func NewDocumentRepository(q Querier) repo.Repository[entity.Document] {
	return &pgRepository[entity.Document]{
		q:       q,
		table:   "documents",
		scanRow: scanDocument,
		toMap:   documentToMap,
	}
}

func assetToMap(a entity.Asset) map[string]any {
	m := make(map[string]any)
	if a.ID != "" {
		m["id"] = a.ID
	}
	if a.TenantID != "" {
		m["tenant_id"] = a.TenantID
	}
	if a.Brand != nil {
		m["brand"] = a.Brand
		if a.NormBrand != nil {
			m["norm_brand"] = a.NormBrand
		} else {
			nb := commons.NormalizeName(*a.Brand)
			m["norm_brand"] = &nb
		}
	}
	if a.Model != nil {
		m["model"] = a.Model
		if a.NormModel != nil {
			m["norm_model"] = a.NormModel
		} else {
			nm := commons.NormalizeName(*a.Model)
			m["norm_model"] = &nm
		}
	}
	if a.SerialNumber != nil {
		m["serial_number"] = a.SerialNumber
		if a.NormSerial != nil {
			m["norm_serial"] = a.NormSerial
		} else {
			ns := commons.NormalizeSerial(*a.SerialNumber)
			m["norm_serial"] = &ns
		}
	}
	// Include explicitly-set norms even if the raw field is nil
	if a.NormSerial != nil && a.SerialNumber == nil {
		m["norm_serial"] = a.NormSerial
	}
	if a.NormBrand != nil && a.Brand == nil {
		m["norm_brand"] = a.NormBrand
	}
	if a.NormModel != nil && a.Model == nil {
		m["norm_model"] = a.NormModel
	}
	if a.PurchaseDate != nil {
		m["purchase_date"] = a.PurchaseDate
	}
	if a.WarrantyEnd != nil {
		m["warranty_end"] = a.WarrantyEnd
	}
	if a.Price != nil {
		m["price"] = a.Price
	}
	if a.Currency != nil {
		m["currency"] = a.Currency
	}
	if a.DocType != "" {
		m["doc_type"] = a.DocType
	}
	if a.Metadata != nil {
		meta, err := toMetadataJSON(a.Metadata)
		if err == nil {
			m["metadata"] = meta
		}
	}
	return m
}

func sourceToMap(s entity.Source) map[string]any {
	m := make(map[string]any)
	if s.ID != "" {
		m["id"] = s.ID
	}
	if s.TenantID != "" {
		m["tenant_id"] = s.TenantID
	}
	if s.Filename != "" {
		m["filename"] = s.Filename
	}
	if s.ContentType != "" {
		m["content_type"] = s.ContentType
	}
	if s.Size > 0 {
		m["byte_size"] = s.Size
	}
	if s.Path != "" {
		m["storage_path"] = s.Path
	}
	if s.SHA256 != "" {
		m["sha256"] = s.SHA256
	}
	if !s.UploadedAt.IsZero() {
		m["uploaded_at"] = s.UploadedAt
	}
	return m
}

func documentToMap(d entity.Document) map[string]any {
	m := make(map[string]any)
	if d.ID != "" {
		m["id"] = d.ID
	}
	if d.TenantID != "" {
		m["tenant_id"] = d.TenantID
	}
	if d.AssetID != "" {
		m["asset_id"] = d.AssetID
	}
	if d.SourceID != "" {
		m["source_id"] = d.SourceID
	}
	if d.DocType != "" {
		m["doc_type"] = d.DocType
	}
	if d.ExtractedFields != nil {
		fields, err := toMetadataJSON(d.ExtractedFields)
		if err == nil {
			m["extracted_fields"] = fields
		}
	}
	if d.RawExtraction != "" {
		raw, err := json.Marshal(d.RawExtraction)
		if err == nil {
			m["raw_extraction"] = raw
		}
	}
	return m
}
