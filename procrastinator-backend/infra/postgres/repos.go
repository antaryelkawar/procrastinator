package postgres

import (
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guards: the concrete asset/source repositories satisfy the
// generic repository interface for their entity.
var (
	_ repo.Repository[entity.Asset]  = (*AssetRepository)(nil)
	_ repo.Repository[entity.Source] = (*SourceRepository)(nil)
)

// AssetRepository is the generic repository engine for entity.Asset, carrying
// the scope-aware ListScoped method.
type AssetRepository struct {
	*pgRepository[entity.Asset]
}

// SourceRepository is the generic repository engine for entity.Source,
// carrying the scope-aware ListScoped method.
type SourceRepository struct {
	*pgRepository[entity.Source]
}

// NewAssetRepository returns a repository for entity.Asset.
func NewAssetRepository(pool *pgxpool.Pool) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:   &poolScope{pool: pool},
			table:   "assets",
			scanRow: scanAsset,
			toMap:   assetToMap,
			filters: assetFilters,
		},
	}
}

// NewSourceRepository returns a repository for entity.Source.
func NewSourceRepository(pool *pgxpool.Pool) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:   &poolScope{pool: pool},
			table:   "sources",
			scanRow: scanSource,
			toMap:   sourceToMap,
			filters: sourceFilters,
		},
	}
}

// NewDocumentRepository returns a repository for entity.Document, carrying the
// document-specific aggregate queries (e.g. LinkCandidates).
func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:   &poolScope{pool: pool},
			table:   "documents",
			scanRow: scanDocument,
			toMap:   documentToMap,
			filters: documentFilters,
		},
	}
}

// newAssetRepoForTx returns an asset repository bound to an ambient transaction.
func newAssetRepoForTx(tx pgx.Tx) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:   &txScopeImpl{tx: tx},
			table:   "assets",
			scanRow: scanAsset,
			toMap:   assetToMap,
			filters: assetFilters,
		},
	}
}

// newSourceRepoForTx returns a source repository bound to an ambient transaction.
func newSourceRepoForTx(tx pgx.Tx) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:   &txScopeImpl{tx: tx},
			table:   "sources",
			scanRow: scanSource,
			toMap:   sourceToMap,
			filters: sourceFilters,
		},
	}
}

// newDocumentRepoForTx returns a document repository bound to an ambient transaction.
func newDocumentRepoForTx(tx pgx.Tx) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:   &txScopeImpl{tx: tx},
			table:   "documents",
			scanRow: scanDocument,
			toMap:   documentToMap,
			filters: documentFilters,
		},
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
	if a.ScopeType != "" {
		m["scope_type"] = a.ScopeType
	}
	if a.OwnerHouseholdID != nil {
		m["owner_household_id"] = a.OwnerHouseholdID
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
	if s.ScopeType != "" {
		m["scope_type"] = s.ScopeType
	}
	if s.OwnerHouseholdID != nil {
		m["owner_household_id"] = s.OwnerHouseholdID
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
	if d.ScopeType != "" {
		m["scope_type"] = d.ScopeType
	}
	if d.OwnerHouseholdID != nil {
		m["owner_household_id"] = d.OwnerHouseholdID
	}
	return m
}

// filterConfig holds the per-entity whitelists for dynamic query names:
// fieldCols maps filterable field names to SQL columns for WHERE clauses,
// orderCols maps orderable field names to SQL columns for ORDER BY clauses.
// Every caller-influenced name is validated against these maps before SQL
// assembly; unknown names are rejected, never interpolated.
type filterConfig struct {
	fieldCols map[string]string
	orderCols map[string]string
}

var assetFieldCols = map[string]string{
	"id":                 "id",
	"brand":              "brand",
	"model":              "model",
	"serial_number":      "serial_number",
	"norm_serial":        "norm_serial",
	"norm_brand":         "norm_brand",
	"norm_model":         "norm_model",
	"purchase_date":      "purchase_date",
	"warranty_end":       "warranty_end",
	"price":              "price",
	"currency":           "currency",
	"doc_type":           "doc_type",
	"created_at":         "created_at",
	"updated_at":         "updated_at",
	"scope_type":         "scope_type",
	"owner_household_id": "owner_household_id",
}

var assetOrderCols = map[string]string{
	"id":            "id",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
	"brand":         "brand",
	"model":         "model",
	"doc_type":      "doc_type",
	"purchase_date": "purchase_date",
	"warranty_end":  "warranty_end",
}

var sourceFieldCols = map[string]string{
	"filename":           "filename",
	"content_type":       "content_type",
	"byte_size":          "byte_size",
	"sha256":             "sha256",
	"uploaded_at":        "uploaded_at",
	"scope_type":         "scope_type",
	"owner_household_id": "owner_household_id",
}

var sourceOrderCols = map[string]string{
	"id":          "id",
	"uploaded_at": "uploaded_at",
	"byte_size":   "byte_size",
}

var documentFieldCols = map[string]string{
	"asset_id":           "asset_id",
	"source_id":          "source_id",
	"doc_type":           "doc_type",
	"created_at":         "created_at",
	"scope_type":         "scope_type",
	"owner_household_id": "owner_household_id",
}

var documentOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"doc_type":   "doc_type",
}

var (
	assetFilters    = filterConfig{fieldCols: assetFieldCols, orderCols: assetOrderCols}
	sourceFilters   = filterConfig{fieldCols: sourceFieldCols, orderCols: sourceOrderCols}
	documentFilters = filterConfig{fieldCols: documentFieldCols, orderCols: documentOrderCols}
)
