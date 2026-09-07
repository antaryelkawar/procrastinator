package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// rowScanner is the subset of pgx.Row / pgx.Rows used for scanning.
type rowScanner interface {
	Scan(dest ...any) error
}

// toMetadataJSON encodes metadata for the jsonb column. nil/empty maps become "{}".
func toMetadataJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// scanAsset scans a row into an entity.Asset, mapping pgx.ErrNoRows to repo.ErrNotFound.
// Column order matches the assets table (migrations 00002 + 00005 + 00006).
// doc_type was dropped in 00006; name/norm_name/asset_category/
// category_confidence/category_user_set/deleted_at/merged_into/merged_at
// were added in 00006.
func scanAsset(row rowScanner) (entity.Asset, error) {
	var a entity.Asset
	var id string
	var metadataJSON []byte
	err := row.Scan(
		&id, &a.OwnerID, &a.Brand, &a.Model, &a.SerialNumber, &a.NormSerial,
		&a.NormBrand, &a.NormModel, &a.PurchaseDate, &a.WarrantyEnd, &a.Price,
		&a.Currency, &metadataJSON, &a.CreatedAt, &a.UpdatedAt,
		&a.OwnerHouseholdID, &a.Confidence,
		&a.Name, &a.NormName, &a.AssetCategory, &a.CategoryConfidence,
		&a.CategoryUserSet, &a.DeletedAt, &a.MergedInto, &a.MergedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Asset{}, repo.ErrNotFound
		}
		return entity.Asset{}, err
	}
	a.ID = id
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &a.Metadata); err != nil {
			return entity.Asset{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return a, nil
}

// scanSource scans a row into an entity.Source, mapping pgx.ErrNoRows to repo.ErrNotFound.
func scanSource(row rowScanner) (entity.Source, error) {
	var s entity.Source
	var id string
	err := row.Scan(
		&id, &s.OwnerID, &s.Filename, &s.ContentType, &s.Size,
		&s.Path, &s.SHA256, &s.UploadedAt,
		&s.OwnerHouseholdID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Source{}, repo.ErrNotFound
		}
		return entity.Source{}, err
	}
	s.ID = id
	return s, nil
}

// scanDocument scans a row into an entity.Document, mapping pgx.ErrNoRows to repo.ErrNotFound.
// Column order matches the documents table (migrations 00002 + 00005).
func scanDocument(row rowScanner) (entity.Document, error) {
	var d entity.Document
	var id string
	var fieldsJSON, rawJSON []byte
	err := row.Scan(
		&id, &d.OwnerID, &d.AssetID, &d.SourceID, &d.DocType,
		&fieldsJSON, &rawJSON, &d.CreatedAt,
		&d.OwnerHouseholdID, &d.Confidence,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Document{}, repo.ErrNotFound
		}
		return entity.Document{}, err
	}
	d.ID = id
	if err := decodeDocumentJSON(fieldsJSON, rawJSON, &d.ExtractedFields, &d.RawExtraction); err != nil {
		return entity.Document{}, err
	}
	return d, nil
}

// decodeDocumentJSON decodes the jsonb columns into the document fields.
// Empty extracted_fields decodes to a nil map; raw_extraction is always
// decoded into a string.
func decodeDocumentJSON(fieldsJSON, rawJSON []byte, fields *map[string]any, raw *string) error {
	if len(fieldsJSON) > 0 {
		if err := json.Unmarshal(fieldsJSON, fields); err != nil {
			return fmt.Errorf("decode extracted_fields: %w", err)
		}
	}
	if len(rawJSON) > 0 {
		if err := json.Unmarshal(rawJSON, raw); err != nil {
			return fmt.Errorf("decode raw_extraction: %w", err)
		}
	}
	return nil
}
