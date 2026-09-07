package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
// the scope-aware (owner + household membership) visibility behavior.
type AssetRepository struct {
	*pgRepository[entity.Asset]
}

// SearchAssets returns the user's assets whose name, brand, model, or
// serial_number case-insensitively contain the (pre-escaped) ILIKE pattern,
// ANDed with any structured filters (category, brand substring, purchase date
// range, warranty status, has-documents), ordered by created_at DESC, id ASC.
// It runs in a scope-bound transaction (app.user_id RLS backstop), applies the
// D-8 visibility rule, and returns a non-nil empty slice when nothing matches.
func (r *AssetRepository) SearchAssets(ctx context.Context, pattern string, filters repo.Filters, opts ...repo.Option) ([]entity.Asset, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.Asset
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)

		var conds []string
		if filters.Category != nil {
			args = append(args, *filters.Category)
			conds = append(conds, fmt.Sprintf("asset_category = $%d", len(args)))
		}
		if filters.Brand != nil {
			args = append(args, "%"+*filters.Brand+"%")
			conds = append(conds, fmt.Sprintf("brand ILIKE $%d", len(args)))
		}
		if filters.PurchaseFrom != nil {
			args = append(args, *filters.PurchaseFrom)
			conds = append(conds, fmt.Sprintf("purchase_date >= $%d", len(args)))
		}
		if filters.PurchaseTo != nil {
			args = append(args, *filters.PurchaseTo)
			conds = append(conds, fmt.Sprintf("purchase_date <= $%d", len(args)))
		}
		if filters.WarrantyStatus != nil {
			switch {
			case *filters.WarrantyStatus == "active":
				conds = append(conds, "warranty_end > now()")
			case *filters.WarrantyStatus == "expired":
				conds = append(conds, "warranty_end < now()")
			case strings.HasPrefix(*filters.WarrantyStatus, "expiring_within:"):
				if days, err := strconv.Atoi(strings.TrimPrefix(*filters.WarrantyStatus, "expiring_within:")); err == nil {
					args = append(args, days)
					conds = append(conds, fmt.Sprintf("warranty_end > now() AND warranty_end <= now() + ($%d * interval '1 day')", len(args)))
				}
			}
		}
		if filters.HasDocuments != nil {
			if *filters.HasDocuments {
				conds = append(conds, "EXISTS (SELECT 1 FROM documents WHERE documents.asset_id = assets.id)")
			} else {
				conds = append(conds, "NOT EXISTS (SELECT 1 FROM documents WHERE documents.asset_id = assets.id)")
			}
		}

		extra := ""
		if len(conds) > 0 {
			extra = " AND " + strings.Join(conds, " AND ")
		}
		stmt := fmt.Sprintf(
			"SELECT * FROM assets WHERE %s AND (name ILIKE $%d OR brand ILIKE $%d OR model ILIKE $%d OR serial_number ILIKE $%d)%s ORDER BY created_at DESC, id ASC",
			vis, patN, patN, patN, patN, extra)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.Asset, 0)
		for rows.Next() {
			item, err := scanAsset(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// SourceRepository is the generic repository engine for entity.Source,
// carrying the scope-aware (owner + household membership) visibility behavior.
type SourceRepository struct {
	*pgRepository[entity.Source]
}

// NewAssetRepository returns a repository for entity.Asset.
func NewAssetRepository(pool *pgxpool.Pool) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:     &poolScope{pool: pool},
			table:     "assets",
			scanRow:   scanAsset,
			toMap:     assetToMap,
			filters:   assetFilters,
			shareable: true,
		},
	}
}

// NewSourceRepository returns a repository for entity.Source.
func NewSourceRepository(pool *pgxpool.Pool) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:     &poolScope{pool: pool},
			table:     "sources",
			scanRow:   scanSource,
			toMap:     sourceToMap,
			filters:   sourceFilters,
			shareable: true,
		},
	}
}

// NewDocumentRepository returns a repository for entity.Document, carrying the
// document-specific aggregate queries (e.g. LinkCandidates).
func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:     &poolScope{pool: pool},
			table:     "documents",
			scanRow:   scanDocument,
			toMap:     documentToMap,
			filters:   documentFilters,
			shareable: true,
		},
	}
}

// newAssetRepoForTx returns an asset repository bound to an ambient transaction.
func newAssetRepoForTx(tx pgx.Tx) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:     &txScopeImpl{tx: tx},
			table:     "assets",
			scanRow:   scanAsset,
			toMap:     assetToMap,
			filters:   assetFilters,
			shareable: true,
		},
	}
}

// newSourceRepoForTx returns a source repository bound to an ambient transaction.
func newSourceRepoForTx(tx pgx.Tx) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:     &txScopeImpl{tx: tx},
			table:     "sources",
			scanRow:   scanSource,
			toMap:     sourceToMap,
			filters:   sourceFilters,
			shareable: true,
		},
	}
}

// newDocumentRepoForTx returns a document repository bound to an ambient transaction.
func newDocumentRepoForTx(tx pgx.Tx) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:     &txScopeImpl{tx: tx},
			table:     "documents",
			scanRow:   scanDocument,
			toMap:     documentToMap,
			filters:   documentFilters,
			shareable: true,
		},
	}
}

func assetToMap(a entity.Asset) map[string]any {
	m := make(map[string]any)
	if a.ID != "" {
		m["id"] = a.ID
	}
	if a.OwnerID != "" {
		m["owner_id"] = a.OwnerID
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
	if a.Metadata != nil {
		meta, err := toMetadataJSON(a.Metadata)
		if err == nil {
			m["metadata"] = meta
		}
	}
	if a.Confidence != nil {
		m["confidence"] = a.Confidence
	}
	if a.OwnerHouseholdID != nil {
		m["owner_household_id"] = a.OwnerHouseholdID
	}
	if a.Name != nil {
		m["name"] = a.Name
		if a.NormName != nil {
			m["norm_name"] = a.NormName
		} else {
			nn := commons.NormalizeName(*a.Name)
			m["norm_name"] = &nn
		}
	}
	if a.NormName != nil && a.Name == nil {
		m["norm_name"] = a.NormName
	}
	if a.AssetCategory != nil {
		m["asset_category"] = a.AssetCategory
	}
	if a.CategoryConfidence != nil {
		m["category_confidence"] = a.CategoryConfidence
	}
	if a.CategoryUserSet {
		m["category_user_set"] = a.CategoryUserSet
	}
	if a.DeletedAt != nil {
		m["deleted_at"] = a.DeletedAt
	}
	if a.MergedInto != nil {
		m["merged_into"] = a.MergedInto
	}
	if a.MergedAt != nil {
		m["merged_at"] = a.MergedAt
	}
	return m
}

func sourceToMap(s entity.Source) map[string]any {
	m := make(map[string]any)
	if s.ID != "" {
		m["id"] = s.ID
	}
	if s.OwnerID != "" {
		m["owner_id"] = s.OwnerID
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
	if d.OwnerID != "" {
		m["owner_id"] = d.OwnerID
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
	if d.Confidence != nil {
		m["confidence"] = d.Confidence
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
	"name":               "name",
	"norm_name":          "norm_name",
	"asset_category":     "asset_category",
	"purchase_date":      "purchase_date",
	"warranty_end":       "warranty_end",
	"price":              "price",
	"currency":           "currency",
	"created_at":         "created_at",
	"updated_at":         "updated_at",
	"deleted_at":         "deleted_at",
	"owner_household_id": "owner_household_id",
}

var assetOrderCols = map[string]string{
	"id":            "id",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
	"brand":         "brand",
	"model":         "model",
	"name":          "name",
	"purchase_date": "purchase_date",
	"warranty_end":  "warranty_end",
}

var sourceFieldCols = map[string]string{
	"id":                 "id",
	"filename":           "filename",
	"content_type":       "content_type",
	"byte_size":          "byte_size",
	"sha256":             "sha256",
	"uploaded_at":        "uploaded_at",
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
