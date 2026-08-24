// Package ingest provides file ingestion, LLM extraction, and identity
// resolution for warranty and invoice documents.
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/identity"
	"procrastinator-backend/internal/llm/extraction"
	"procrastinator-backend/internal/store/sources"
)

// ErrExtraction is returned when the LLM extraction call fails or the
// response cannot be parsed as a valid extraction payload.
var ErrExtraction = errors.New("llm extraction failed")

// ErrTooLarge is returned when an upload exceeds the configured size limit.
var ErrTooLarge = errors.New("upload exceeds size limit")

// Extractor extracts structured JSON data from raw file bytes.
type Extractor interface {
	Extract(ctx context.Context, contentType string, data []byte) ([]byte, error)
}

// Service orchestrates the full ingestion pipeline: size validation, file
// storage, source persistence, LLM extraction, identity resolution, and
// document creation.
type Service struct {
	pool      *pgxpool.Pool
	storage   *Storage
	srcRepo   *sources.Repo
	extractor Extractor
	maxBytes  int64
}

// NewService creates a Service rooted at cfg.StorageDir with the given
// database pool and extractor. If cfg.MaxUploadBytes is non-positive, the
// default limit of 20 MiB (20971520 bytes) is used.
func NewService(cfg config.Config, pool *pgxpool.Pool, ex Extractor) *Service {
	maxBytes := cfg.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 20971520 // 20 MiB
	}
	return &Service{
		pool:      pool,
		storage:   NewStorage(cfg.StorageDir),
		srcRepo:   sources.New(pool),
		extractor: ex,
		maxBytes:  maxBytes,
	}
}

// Process runs the full ingestion pipeline for a single upload. It persists
// the source file and row before invoking the LLM; downstream failures
// (extraction, identity resolution) do not remove the source.
func (s *Service) Process(ctx context.Context, filename string, data []byte) (identity.Asset, error) {
	// 1. Oversize check — nothing persisted.
	if int64(len(data)) > s.maxBytes {
		return identity.Asset{}, fmt.Errorf("ingest: %w: %d bytes exceeds limit %d", ErrTooLarge, len(data), s.maxBytes)
	}

	// 2. Sniff + store — sniffs content type before writing.
	sf, err := s.storage.Put(data)
	if err != nil {
		return identity.Asset{}, err
	}

	// 3. Source row BEFORE any LLM call. Survives all downstream failures.
	src, err := s.srcRepo.Create(ctx, sources.Source{
		Filename:    filename,
		ContentType: sf.ContentType,
		ByteSize:    sf.ByteSize,
		StoragePath: sf.Path,
		SHA256:      sf.SHA256,
	})
	if err != nil {
		return identity.Asset{}, fmt.Errorf("ingest: create source: %w", err)
	}

	// 4. LLM extraction.
	raw, err := s.extractor.Extract(ctx, sf.ContentType, data)
	if err != nil {
		return identity.Asset{}, fmt.Errorf("ingest: %w: %v", ErrExtraction, err)
	}

	// 5. Parse extraction payload.
	ex, err := extraction.ParseExtraction(raw)
	if err != nil {
		return identity.Asset{}, fmt.Errorf("ingest: %w: %v", ErrExtraction, err)
	}

	// 6. Transactional identity resolution + document insert.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return identity.Asset{}, fmt.Errorf("ingest: begin tx: %w", err)
	}

	asset, _, err := identity.Resolve(ctx, &txAssetStore{tx: tx}, ex)
	if err != nil {
		_ = tx.Rollback(ctx)
		return identity.Asset{}, fmt.Errorf("ingest: %w", err)
	}

	// Build extracted_fields JSON (present values only).
	ef := buildExtractedFields(ex)
	efJSON, err := json.Marshal(ef)
	if err != nil {
		_ = tx.Rollback(ctx)
		return identity.Asset{}, fmt.Errorf("ingest: marshal extracted_fields: %w", err)
	}

	// Build raw_extraction as a JSON string (quoted, with <thought> blocks).
	rawJSON, err := json.Marshal(ex.RawPayload)
	if err != nil {
		_ = tx.Rollback(ctx)
		return identity.Asset{}, fmt.Errorf("ingest: marshal raw_extraction: %w", err)
	}

	// INSERT document within the same transaction.
	const insertDoc = `INSERT INTO documents (asset_id, source_id, doc_type, extracted_fields, raw_extraction) VALUES ($1,$2,$3,$4,$5)`
	if _, err := tx.Exec(ctx, insertDoc,
		asset.ID, src.ID, ex.Classification,
		string(efJSON), string(rawJSON),
	); err != nil {
		_ = tx.Rollback(ctx)
		return identity.Asset{}, fmt.Errorf("ingest: insert document: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return identity.Asset{}, fmt.Errorf("ingest: commit: %w", err)
	}

	return asset, nil
}

// extractedFields is the JSON shape stored in the documents.extracted_fields
// column. Only non-nil fields are emitted (omitempty).
type extractedFields struct {
	DocumentType  string  `json:"document_type"`
	Brand         *string `json:"brand,omitempty"`
	Model         *string `json:"model,omitempty"`
	SerialNumber  *string `json:"serial_number,omitempty"`
	PurchaseDate  *string `json:"purchase_date,omitempty"`
	Price         *string `json:"price,omitempty"`
	Currency      *string `json:"currency,omitempty"`
	WarrantyStart *string `json:"warranty_start,omitempty"`
	WarrantyEnd   *string `json:"warranty_end,omitempty"`
}

// buildExtractedFields populates an extractedFields from a parsed extraction.
// String fields are stored verbatim; dates are formatted as YYYY-MM-DD.
func buildExtractedFields(ex extraction.Extraction) extractedFields {
	ef := extractedFields{DocumentType: ex.Classification}
	if ex.Brand != "" {
		s := ex.Brand
		ef.Brand = &s
	}
	if ex.Model != "" {
		s := ex.Model
		ef.Model = &s
	}
	if ex.SerialNumber != "" {
		s := ex.SerialNumber
		ef.SerialNumber = &s
	}
	if !ex.PurchaseDate.IsZero() {
		s := ex.PurchaseDate.Format(time.DateOnly)
		ef.PurchaseDate = &s
	}
	if ex.Price != "" {
		s := ex.Price
		ef.Price = &s
	}
	if ex.Currency != "" {
		s := ex.Currency
		ef.Currency = &s
	}
	if !ex.WarrantyStart.IsZero() {
		s := ex.WarrantyStart.Format(time.DateOnly)
		ef.WarrantyStart = &s
	}
	if !ex.WarrantyEnd.IsZero() {
		s := ex.WarrantyEnd.Format(time.DateOnly)
		ef.WarrantyEnd = &s
	}
	return ef
}

// ---------------------------------------------------------------------------
// txAssetStore — identity.AssetStore over pgx.Tx
// ---------------------------------------------------------------------------

// txAssetStore implements identity.AssetStore directly over a pgx.Tx so that
// identity resolution and document insertion share a single database
// transaction (design D6).
type txAssetStore struct {
	tx pgx.Tx
}

// assetSelectColumns mirrors the column list in internal/store/assets.
const assetSelectColumns = `id, brand, model, serial_number, purchase_date, price, currency, warranty_start, warranty_end, norm_brand, norm_model, norm_serial, created_at, updated_at`

// scanAssetRow decodes the standard 14-column asset row into an identity.Asset.
// Norm columns are read but discarded.
func scanAssetRow(row pgx.Row) (identity.Asset, error) {
	var a identity.Asset
	var nb, nm, ns *string
	err := row.Scan(
		&a.ID, &a.Brand, &a.Model, &a.SerialNumber,
		&a.PurchaseDate, &a.Price, &a.Currency,
		&a.WarrantyStart, &a.WarrantyEnd,
		&nb, &nm, &ns,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return identity.Asset{}, err
	}
	return a, nil
}

// normCol returns the normalized string when the source pointer is non-nil
// and the normalized result is non-empty; otherwise returns nil. This mirrors
// the normValue semantics in internal/store/assets.
func normCol(src *string, normalize func(string) string) any {
	if src == nil {
		return nil
	}
	n := normalize(*src)
	if n == "" {
		return nil
	}
	return n
}

// FindBySerial looks up an asset by normalized serial number. The input is
// re-normalized via identity.NormalizeSerial (idempotent).
func (s *txAssetStore) FindBySerial(ctx context.Context, normSerial string) (identity.Asset, bool, error) {
	norm := identity.NormalizeSerial(normSerial)
	q := `SELECT ` + assetSelectColumns + ` FROM assets WHERE norm_serial = $1`
	a, err := scanAssetRow(s.tx.QueryRow(ctx, q, norm))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Asset{}, false, nil
		}
		return identity.Asset{}, false, err
	}
	return a, true, nil
}

// FindByBrandModel looks up an asset by normalized brand and model. Both
// inputs are re-normalized via identity.NormalizeName (idempotent).
func (s *txAssetStore) FindByBrandModel(ctx context.Context, normBrand, normModel string) (identity.Asset, bool, error) {
	nb := identity.NormalizeName(normBrand)
	nm := identity.NormalizeName(normModel)
	q := `SELECT ` + assetSelectColumns + ` FROM assets WHERE norm_brand = $1 AND norm_model = $2`
	a, err := scanAssetRow(s.tx.QueryRow(ctx, q, nb, nm))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Asset{}, false, nil
		}
		return identity.Asset{}, false, err
	}
	return a, true, nil
}

// Create inserts a new asset and returns it with store-assigned fields (id,
// timestamps). Norm columns are derived from the source values.
func (s *txAssetStore) Create(ctx context.Context, a identity.Asset) (identity.Asset, error) {
	const q = `INSERT INTO assets (
		brand, model, serial_number, purchase_date, price, currency,
		warranty_start, warranty_end,
		norm_brand, norm_model, norm_serial
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	RETURNING ` + assetSelectColumns

	row := s.tx.QueryRow(ctx, q,
		a.Brand, a.Model, a.SerialNumber,
		a.PurchaseDate, a.Price, a.Currency,
		a.WarrantyStart, a.WarrantyEnd,
		normCol(a.Brand, identity.NormalizeName),
		normCol(a.Model, identity.NormalizeName),
		normCol(a.SerialNumber, identity.NormalizeSerial),
	)
	return scanAssetRow(row)
}

// CreateOnConflictSerial inserts a new asset atomically. On normalized-serial
// conflict it re-selects and returns the existing row with created=false.
func (s *txAssetStore) CreateOnConflictSerial(ctx context.Context, a identity.Asset) (identity.Asset, bool, error) {
	const q = `INSERT INTO assets (
		brand, model, serial_number, purchase_date, price, currency,
		warranty_start, warranty_end,
		norm_brand, norm_model, norm_serial
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	ON CONFLICT (norm_serial) WHERE norm_serial IS NOT NULL DO NOTHING
	RETURNING ` + assetSelectColumns

	row := s.tx.QueryRow(ctx, q,
		a.Brand, a.Model, a.SerialNumber,
		a.PurchaseDate, a.Price, a.Currency,
		a.WarrantyStart, a.WarrantyEnd,
		normCol(a.Brand, identity.NormalizeName),
		normCol(a.Model, identity.NormalizeName),
		normCol(a.SerialNumber, identity.NormalizeSerial),
	)
	inserted, scanErr := scanAssetRow(row)
	if scanErr == nil {
		return inserted, true, nil
	}
	if !errors.Is(scanErr, pgx.ErrNoRows) {
		return identity.Asset{}, false, scanErr
	}

	// Conflict — re-select existing row by normalized serial.
	if a.SerialNumber == nil {
		return identity.Asset{}, false, fmt.Errorf("ingest: CreateOnConflictSerial: serial_number is nil on conflict path")
	}
	existing, ok, findErr := s.FindBySerial(ctx, *a.SerialNumber)
	if findErr != nil {
		return identity.Asset{}, false, findErr
	}
	if !ok {
		return identity.Asset{}, false, fmt.Errorf("ingest: CreateOnConflictSerial: conflict detected but no existing row found")
	}
	return existing, false, nil
}

// UpdateFields applies only non-nil fields to the asset identified by id.
// When all fields are nil the current row is returned unchanged. Brand,
// Model, and SerialNumber also update their corresponding norm columns.
// updated_at is always bumped when any field is set.
func (s *txAssetStore) UpdateFields(ctx context.Context, id string, f identity.UpdateFields) (identity.Asset, error) {
	anySet := f.Brand != nil || f.Model != nil || f.SerialNumber != nil ||
		f.PurchaseDate != nil || f.Price != nil || f.Currency != nil ||
		f.WarrantyStart != nil || f.WarrantyEnd != nil

	if !anySet {
		q := `SELECT ` + assetSelectColumns + ` FROM assets WHERE id = $1`
		a, err := scanAssetRow(s.tx.QueryRow(ctx, q, id))
		if err != nil {
			return identity.Asset{}, err
		}
		return a, nil
	}

	set := make([]string, 0, 9)
	args := make([]any, 0, 10)
	idx := 1

	add := func(col string, v any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, v)
		idx++
	}

	if f.Brand != nil {
		add("brand", f.Brand)
		add("norm_brand", normCol(f.Brand, identity.NormalizeName))
	}
	if f.Model != nil {
		add("model", f.Model)
		add("norm_model", normCol(f.Model, identity.NormalizeName))
	}
	if f.SerialNumber != nil {
		add("serial_number", f.SerialNumber)
		add("norm_serial", normCol(f.SerialNumber, identity.NormalizeSerial))
	}
	if f.PurchaseDate != nil {
		add("purchase_date", f.PurchaseDate)
	}
	if f.Price != nil {
		add("price", f.Price)
	}
	if f.Currency != nil {
		add("currency", f.Currency)
	}
	if f.WarrantyStart != nil {
		add("warranty_start", f.WarrantyStart)
	}
	if f.WarrantyEnd != nil {
		add("warranty_end", f.WarrantyEnd)
	}

	set = append(set, "updated_at = now()")
	args = append(args, id)

	q := `UPDATE assets SET ` + strings.Join(set, ", ") + ` WHERE id = $` + fmt.Sprintf("%d", idx) + ` RETURNING ` + assetSelectColumns
	a, err := scanAssetRow(s.tx.QueryRow(ctx, q, args...))
	if err != nil {
		return identity.Asset{}, err
	}
	return a, nil
}
