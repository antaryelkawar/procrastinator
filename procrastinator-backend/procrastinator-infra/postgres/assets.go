package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons-data"
)

var _ data.AssetRepository = (*pgAssetRepo)(nil)

// pgAssetRepo implements data.AssetRepository on top of a pgx Querier.
type pgAssetRepo struct {
	q Querier
}

// NewAssetRepo returns an asset repository bound to q.
func NewAssetRepo(q Querier) *pgAssetRepo {
	return &pgAssetRepo{q: q}
}

const assetColumns = `id, tenant_id, brand, model, serial_number, norm_serial,
	norm_brand, norm_model, purchase_date, warranty_end, price, currency,
	doc_type, metadata, created_at, updated_at`

// deriveNorms fills the norm columns from the raw values. The input is not
// mutated; the returned value carries the computed norms.
func deriveNorms(a data.Asset) data.Asset {
	if a.SerialNumber != nil {
		ns := data.NormalizeSerial(*a.SerialNumber)
		a.NormSerial = &ns
	}
	if a.Brand != nil {
		nb := data.NormalizeName(*a.Brand)
		a.NormBrand = &nb
	}
	if a.Model != nil {
		nm := data.NormalizeName(*a.Model)
		a.NormModel = &nm
	}
	return a
}

// toMetadataJSON encodes metadata for the jsonb column. nil/empty maps become "{}".
func toMetadataJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func (r *pgAssetRepo) Create(ctx context.Context, a data.Asset) (data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, err
	}
	a.TenantID = tenant
	a = deriveNorms(a)
	meta, err := toMetadataJSON(a.Metadata)
	if err != nil {
		return data.Asset{}, fmt.Errorf("encode metadata: %w", err)
	}

	const stmt = `
		INSERT INTO assets (
			id, tenant_id, brand, model, serial_number, norm_serial,
			norm_brand, norm_model, purchase_date, warranty_end, price,
			currency, doc_type, metadata
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING ` + assetColumns
	row := r.q.QueryRow(ctx, stmt,
		a.TenantID, a.Brand, a.Model, a.SerialNumber, a.NormSerial,
		a.NormBrand, a.NormModel, a.PurchaseDate, a.WarrantyEnd, a.Price,
		a.Currency, a.DocType, meta,
	)
	return scanAsset(row)
}

func (r *pgAssetRepo) CreateOnConflictSerial(ctx context.Context, a data.Asset) (data.Asset, bool, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, false, err
	}
	if a.NormSerial == nil && a.SerialNumber != nil {
		a = deriveNorms(a)
	}
	if a.NormSerial == nil {
		// No serial to conflict on: plain insert.
		created, err := r.Create(ctx, a)
		return created, true, err
	}
	a.TenantID = tenant
	a = deriveNorms(a)
	meta, err := toMetadataJSON(a.Metadata)
	if err != nil {
		return data.Asset{}, false, fmt.Errorf("encode metadata: %w", err)
	}

	const stmt = `
		INSERT INTO assets (
			id, tenant_id, brand, model, serial_number, norm_serial,
			norm_brand, norm_model, purchase_date, warranty_end, price,
			currency, doc_type, metadata
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (tenant_id, norm_serial) WHERE norm_serial IS NOT NULL DO NOTHING
		RETURNING ` + assetColumns
	created, err := scanAsset(r.q.QueryRow(ctx, stmt,
		a.TenantID, a.Brand, a.Model, a.SerialNumber, a.NormSerial,
		a.NormBrand, a.NormModel, a.PurchaseDate, a.WarrantyEnd, a.Price,
		a.Currency, a.DocType, meta,
	))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, data.ErrNotFound) {
		return data.Asset{}, false, err
	}

	// Conflict: fetch the existing row.
	existing, err := r.FindBySerial(ctx, *a.NormSerial)
	if err != nil {
		return data.Asset{}, false, err
	}
	return existing, false, nil
}

func (r *pgAssetRepo) GetByID(ctx context.Context, id string) (data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, err
	}
	const stmt = `SELECT ` + assetColumns + ` FROM assets WHERE id = $1 AND tenant_id = $2`
	return scanAsset(r.q.QueryRow(ctx, stmt, id, tenant))
}

func (r *pgAssetRepo) List(ctx context.Context) ([]data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return nil, err
	}
	const stmt = `SELECT ` + assetColumns + ` FROM assets WHERE tenant_id = $1 ORDER BY created_at, id`
	rows, err := r.q.Query(ctx, stmt, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := make([]data.Asset, 0)
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *pgAssetRepo) FindBySerial(ctx context.Context, normSerial string) (data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, err
	}
	const stmt = `SELECT ` + assetColumns + ` FROM assets WHERE tenant_id = $1 AND norm_serial = $2`
	return scanAsset(r.q.QueryRow(ctx, stmt, tenant, normSerial))
}

func (r *pgAssetRepo) FindByBrandModel(ctx context.Context, normBrand, normModel string) (data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, err
	}
	const stmt = `SELECT ` + assetColumns + ` FROM assets WHERE tenant_id = $1 AND norm_brand = $2 AND norm_model = $3`
	return scanAsset(r.q.QueryRow(ctx, stmt, tenant, normBrand, normModel))
}

func (r *pgAssetRepo) UpdateFields(ctx context.Context, id string, f data.UpdateFields) (data.Asset, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Asset{}, err
	}
	if !hasAnyUpdateField(f) {
		return r.GetByID(ctx, id)
	}

	var setClauses []string
	var args []any
	add := func(clause string, arg any) {
		args = append(args, arg)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", clause, len(args)))
	}

	if f.Brand != nil {
		add("brand", *f.Brand)
		add("norm_brand", data.NormalizeName(*f.Brand))
	}
	if f.Model != nil {
		add("model", *f.Model)
		add("norm_model", data.NormalizeName(*f.Model))
	}
	if f.SerialNumber != nil {
		add("serial_number", *f.SerialNumber)
		add("norm_serial", data.NormalizeSerial(*f.SerialNumber))
	}
	if f.PurchaseDate != nil {
		add("purchase_date", *f.PurchaseDate)
	}
	if f.WarrantyEnd != nil {
		add("warranty_end", *f.WarrantyEnd)
	}
	if f.Price != nil {
		add("price", *f.Price)
	}
	if f.Currency != nil {
		add("currency", *f.Currency)
	}
	if f.DocType != nil {
		add("doc_type", *f.DocType)
	}
	if f.Metadata != nil {
		meta, err := toMetadataJSON(f.Metadata)
		if err != nil {
			return data.Asset{}, fmt.Errorf("encode metadata: %w", err)
		}
		args = append(args, meta)
		setClauses = append(setClauses, fmt.Sprintf("metadata = metadata || $%d::jsonb", len(args)))
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, id, tenant)

	stmt := fmt.Sprintf(
		`UPDATE assets SET %s WHERE id = $%d AND tenant_id = $%d RETURNING `+assetColumns,
		strings.Join(setClauses, ", "), len(args)-1, len(args),
	)
	return scanAsset(r.q.QueryRow(ctx, stmt, args...))
}

func hasAnyUpdateField(f data.UpdateFields) bool {
	return f.Brand != nil || f.Model != nil || f.SerialNumber != nil ||
		f.PurchaseDate != nil || f.WarrantyEnd != nil || f.Price != nil ||
		f.Currency != nil || f.DocType != nil || f.Metadata != nil
}

// scanAsset scans a single asset row from any pgx.Rows or pgx.Row (both
// expose Scan with the same semantics). pgx.ErrNoRows is mapped to
// data.ErrNotFound so callers can use errors.Is uniformly.
func scanAsset(row rowScanner) (data.Asset, error) {
	var a data.Asset
	var id string
	var metadataJSON []byte
	err := row.Scan(
		&id, &a.TenantID, &a.Brand, &a.Model, &a.SerialNumber, &a.NormSerial,
		&a.NormBrand, &a.NormModel, &a.PurchaseDate, &a.WarrantyEnd, &a.Price,
		&a.Currency, &a.DocType, &metadataJSON, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return data.Asset{}, data.ErrNotFound
		}
		return data.Asset{}, err
	}
	a.ID = id
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &a.Metadata); err != nil {
			return data.Asset{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return a, nil
}

// rowScanner is the subset of pgx.Row / pgx.Rows used for scanning.
type rowScanner interface {
	Scan(dest ...any) error
}
