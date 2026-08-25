package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons-data"
)

var _ data.SourceRepository = (*pgSourceRepo)(nil)

// pgSourceRepo implements data.SourceRepository on top of a pgx Querier.
type pgSourceRepo struct {
	q Querier
}

// NewSourceRepo returns a source repository bound to q.
func NewSourceRepo(q Querier) *pgSourceRepo {
	return &pgSourceRepo{q: q}
}

const sourceColumns = `id, tenant_id, filename, content_type, byte_size,
	storage_path, sha256, uploaded_at`

func (r *pgSourceRepo) Create(ctx context.Context, s data.Source) (data.Source, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Source{}, err
	}
	s.TenantID = tenant

	// A zero UploadedAt is passed as NULL so COALESCE substitutes now().
	var uploadedAt *time.Time
	if !s.UploadedAt.IsZero() {
		uploadedAt = &s.UploadedAt
	}

	const stmt = `
		INSERT INTO sources (
			id, tenant_id, filename, content_type, byte_size,
			storage_path, sha256, uploaded_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, COALESCE($7, now()))
		RETURNING ` + sourceColumns
	row := r.q.QueryRow(ctx, stmt,
		s.TenantID, s.Filename, s.ContentType, s.Size,
		s.Path, s.SHA256, uploadedAt,
	)
	return scanSource(row)
}

func (r *pgSourceRepo) GetByID(ctx context.Context, id string) (data.Source, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Source{}, err
	}
	const stmt = `SELECT ` + sourceColumns + ` FROM sources WHERE id = $1 AND tenant_id = $2`
	return scanSource(r.q.QueryRow(ctx, stmt, id, tenant))
}

// scanSource scans a single source row from any pgx.Rows or pgx.Row (both
// expose Scan with the same semantics). pgx.ErrNoRows is mapped to
// data.ErrNotFound so callers can use errors.Is uniformly.
func scanSource(row rowScanner) (data.Source, error) {
	var s data.Source
	var id string
	err := row.Scan(
		&id, &s.TenantID, &s.Filename, &s.ContentType, &s.Size,
		&s.Path, &s.SHA256, &s.UploadedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return data.Source{}, data.ErrNotFound
		}
		return data.Source{}, err
	}
	s.ID = id
	return s, nil
}
