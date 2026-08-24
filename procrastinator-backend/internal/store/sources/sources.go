// Package sources implements the sources repository backed by PostgreSQL.
package sources

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound indicates a source did not exist for the requested operation.
var ErrNotFound = errors.New("source not found")

// Source is the in-memory representation of a sources row.
type Source struct {
	ID          string
	Filename    string
	ContentType string
	ByteSize    int64
	StoragePath string
	SHA256      string
	UploadedAt  time.Time
}

// Repo stores and retrieves sources.
type Repo struct {
	pool *pgxpool.Pool
}

// New returns a Repo backed by the provided connection pool.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const selectColumns = `id, filename, content_type, byte_size, storage_path, sha256, uploaded_at`

// scanRow decodes the standard source column set from a pgx.Row.
func scanRow(row pgx.Row) (Source, error) {
	var s Source
	err := row.Scan(
		&s.ID, &s.Filename, &s.ContentType, &s.ByteSize,
		&s.StoragePath, &s.SHA256, &s.UploadedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Source{}, ErrNotFound
		}
		return Source{}, err
	}
	return s, nil
}

// Create inserts a new source and returns the inserted row.
func (r *Repo) Create(ctx context.Context, s Source) (Source, error) {
	const q = `INSERT INTO sources (
		filename, content_type, byte_size, storage_path, sha256
	) VALUES ($1,$2,$3,$4,$5)
	RETURNING ` + selectColumns

	row, err := scanRow(r.pool.QueryRow(ctx, q,
		s.Filename, s.ContentType, s.ByteSize, s.StoragePath, s.SHA256,
	))
	if err != nil {
		return Source{}, fmt.Errorf("sources: create: %w", err)
	}
	return row, nil
}

// GetByID retrieves a source by its primary key.
func (r *Repo) GetByID(ctx context.Context, id string) (Source, error) {
	q := `SELECT ` + selectColumns + ` FROM sources WHERE id = $1`
	s, err := scanRow(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Source{}, fmt.Errorf("sources: get by id: %w", ErrNotFound)
		}
		return Source{}, fmt.Errorf("sources: get by id: %w", err)
	}
	return s, nil
}
