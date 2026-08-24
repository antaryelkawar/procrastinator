// Package assets implements the assets repository backed by PostgreSQL.
package assets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound indicates an asset did not exist for the requested operation.
var ErrNotFound = errors.New("asset not found")

// Asset is the in-memory representation of an assets row.
type Asset struct {
	ID            string
	Brand         *string
	Model         *string
	SerialNumber  *string
	PurchaseDate  *time.Time
	Price         *string
	Currency      *string
	WarrantyStart *time.Time
	WarrantyEnd   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UpdateFields describes which fields to mutate on an existing asset.
// Nil fields are left untouched; non-nil fields are applied (including
// the corresponding norm columns for Brand, Model, and SerialNumber).
type UpdateFields struct {
	Brand         *string
	Model         *string
	SerialNumber  *string
	PurchaseDate  *time.Time
	Price         *string
	Currency      *string
	WarrantyStart *time.Time
	WarrantyEnd   *time.Time
}

// Repo stores and retrieves assets.
type Repo struct {
	pool *pgxpool.Pool
}

// New returns a Repo backed by the provided connection pool.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const selectColumns = `id, brand, model, serial_number, purchase_date, price, currency, warranty_start, warranty_end, norm_brand, norm_model, norm_serial, created_at, updated_at`

// scanRow decodes the standard asset column set from a pgx.Row.
// Norm columns are discarded since they are not exposed on Asset.
func scanRow(row pgx.Row) (Asset, error) {
	var a Asset
	var nb, nm, ns *string
	err := row.Scan(
		&a.ID, &a.Brand, &a.Model, &a.SerialNumber,
		&a.PurchaseDate, &a.Price, &a.Currency,
		&a.WarrantyStart, &a.WarrantyEnd,
		&nb, &nm, &ns,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, ErrNotFound
		}
		return Asset{}, err
	}
	return a, nil
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalizeSerial collapses whitespace, trims, and upper-cases a serial.
// Returns ("", false) when the input is nil or collapses to empty.
func normalizeSerial(s *string) (string, bool) {
	if s == nil {
		return "", false
	}
	n := collapseWhitespace(*s)
	if n == "" {
		return "", false
	}
	return strings.ToUpper(n), true
}

// normalizeName collapses whitespace, trims, and lower-cases a name.
// Returns ("", false) when the input is nil or collapses to empty.
func normalizeName(s *string) (string, bool) {
	if s == nil {
		return "", false
	}
	n := collapseWhitespace(*s)
	if n == "" {
		return "", false
	}
	return strings.ToLower(n), true
}

// normValue returns nil when the source was nil or normalized to empty,
// otherwise returns the normalized string.
func normValue(src *string, normalized string, hasNormalized bool) any {
	if src == nil || !hasNormalized || normalized == "" {
		return nil
	}
	return normalized
}

// Create inserts a new asset and returns the inserted row.
func (r *Repo) Create(ctx context.Context, a Asset) (Asset, error) {
	nb, nbOK := normalizeName(a.Brand)
	nm, nmOK := normalizeName(a.Model)
	ns, nsOK := normalizeSerial(a.SerialNumber)

	const q = `INSERT INTO assets (
		brand, model, serial_number, purchase_date, price, currency,
		warranty_start, warranty_end,
		norm_brand, norm_model, norm_serial
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	RETURNING ` + selectColumns

	row, err := scanRow(r.pool.QueryRow(ctx, q,
		a.Brand, a.Model, a.SerialNumber,
		a.PurchaseDate, a.Price, a.Currency,
		a.WarrantyStart, a.WarrantyEnd,
		normValue(a.Brand, nb, nbOK),
		normValue(a.Model, nm, nmOK),
		normValue(a.SerialNumber, ns, nsOK),
	))
	if err != nil {
		return Asset{}, fmt.Errorf("assets: create: %w", err)
	}
	return row, nil
}

// CreateOnConflictSerial inserts the asset, or returns the existing row
// with the same normalized serial when one already exists. The second return
// value indicates whether the insert actually happened.
func (r *Repo) CreateOnConflictSerial(ctx context.Context, a Asset) (Asset, bool, error) {
	nb, nbOK := normalizeName(a.Brand)
	nm, nmOK := normalizeName(a.Model)
	ns, nsOK := normalizeSerial(a.SerialNumber)

	const q = `INSERT INTO assets (
		brand, model, serial_number, purchase_date, price, currency,
		warranty_start, warranty_end,
		norm_brand, norm_model, norm_serial
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	ON CONFLICT (norm_serial) WHERE norm_serial IS NOT NULL DO NOTHING
	RETURNING ` + selectColumns

	inserted, err := scanRow(r.pool.QueryRow(ctx, q,
		a.Brand, a.Model, a.SerialNumber,
		a.PurchaseDate, a.Price, a.Currency,
		a.WarrantyStart, a.WarrantyEnd,
		normValue(a.Brand, nb, nbOK),
		normValue(a.Model, nm, nmOK),
		normValue(a.SerialNumber, ns, nsOK),
	))
	if err == nil {
		return inserted, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Asset{}, false, fmt.Errorf("assets: create on conflict: %w", err)
	}

	// Conflict — re-select the existing row by normalized serial.
	existing, err := r.FindBySerial(ctx, *a.SerialNumber)
	if err != nil {
		return Asset{}, false, fmt.Errorf("assets: create on conflict: %w", err)
	}
	return existing, false, nil
}

// GetByID retrieves an asset by its primary key.
func (r *Repo) GetByID(ctx context.Context, id string) (Asset, error) {
	q := `SELECT ` + selectColumns + ` FROM assets WHERE id = $1`
	a, err := scanRow(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Asset{}, fmt.Errorf("assets: get by id: %w", ErrNotFound)
		}
		return Asset{}, fmt.Errorf("assets: get by id: %w", err)
	}
	return a, nil
}

// List returns all assets ordered by created_at then id.
func (r *Repo) List(ctx context.Context) ([]Asset, error) {
	q := `SELECT ` + selectColumns + ` FROM assets ORDER BY created_at, id`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("assets: list: %w", err)
	}
	defer rows.Close()

	out := make([]Asset, 0)
	for rows.Next() {
		var a Asset
		var nb, nm, ns *string
		if err := rows.Scan(
			&a.ID, &a.Brand, &a.Model, &a.SerialNumber,
			&a.PurchaseDate, &a.Price, &a.Currency,
			&a.WarrantyStart, &a.WarrantyEnd,
			&nb, &nm, &ns,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("assets: list scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("assets: list rows: %w", err)
	}
	return out, nil
}

// FindBySerial looks up an asset by its (normalized) serial number.
func (r *Repo) FindBySerial(ctx context.Context, serial string) (Asset, error) {
	norm := strings.ToUpper(collapseWhitespace(serial))
	q := `SELECT ` + selectColumns + ` FROM assets WHERE norm_serial = $1`
	a, err := scanRow(r.pool.QueryRow(ctx, q, norm))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Asset{}, fmt.Errorf("assets: find by serial: %w", ErrNotFound)
		}
		return Asset{}, fmt.Errorf("assets: find by serial: %w", err)
	}
	return a, nil
}

// FindByBrandModel looks up an asset by normalized brand and model.
func (r *Repo) FindByBrandModel(ctx context.Context, brand, model string) (Asset, error) {
	nb := strings.ToLower(collapseWhitespace(brand))
	nm := strings.ToLower(collapseWhitespace(model))
	q := `SELECT ` + selectColumns + ` FROM assets WHERE norm_brand = $1 AND norm_model = $2`
	a, err := scanRow(r.pool.QueryRow(ctx, q, nb, nm))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Asset{}, fmt.Errorf("assets: find by brand/model: %w", ErrNotFound)
		}
		return Asset{}, fmt.Errorf("assets: find by brand/model: %w", err)
	}
	return a, nil
}

// UpdateFields applies only non-nil fields of f to the asset identified by id.
// If all fields of f are nil, the current row is returned unchanged (updated_at
// is NOT bumped). Returns ErrNotFound when id does not exist.
func (r *Repo) UpdateFields(ctx context.Context, id string, f UpdateFields) (Asset, error) {
	anySet := f.Brand != nil || f.Model != nil || f.SerialNumber != nil ||
		f.PurchaseDate != nil || f.Price != nil || f.Currency != nil ||
		f.WarrantyStart != nil || f.WarrantyEnd != nil

	if !anySet {
		return r.GetByID(ctx, id)
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
		nb, ok := normalizeName(f.Brand)
		add("norm_brand", normValue(f.Brand, nb, ok))
	}
	if f.Model != nil {
		add("model", f.Model)
		nm, ok := normalizeName(f.Model)
		add("norm_model", normValue(f.Model, nm, ok))
	}
	if f.SerialNumber != nil {
		add("serial_number", f.SerialNumber)
		ns, ok := normalizeSerial(f.SerialNumber)
		add("norm_serial", normValue(f.SerialNumber, ns, ok))
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

	q := `UPDATE assets SET ` + strings.Join(set, ", ") + ` WHERE id = $` + fmt.Sprintf("%d", idx) + ` RETURNING ` + selectColumns
	a, err := scanRow(r.pool.QueryRow(ctx, q, args...))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Asset{}, fmt.Errorf("assets: update fields: %w", ErrNotFound)
		}
		return Asset{}, fmt.Errorf("assets: update fields: %w", err)
	}
	return a, nil
}
