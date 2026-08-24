// Package documents implements the documents repository backed by PostgreSQL.
package documents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocType represents the kind of document stored.
type DocType string

// Valid document type constants.
const (
	TypeInvoice  DocType = "invoice"
	TypeWarranty DocType = "warranty"
	TypeOther    DocType = "other"
)

// Valid reports whether d is one of the accepted document type constants.
func (d DocType) Valid() bool {
	return d == TypeInvoice || d == TypeWarranty || d == TypeOther
}

// ErrNotFound indicates a document did not exist for the requested operation.
var ErrNotFound = errors.New("document not found")

// ErrInvalidDocType indicates the document type is not one of the accepted constants.
var ErrInvalidDocType = errors.New("invalid document type")

// ErrSourceAlreadyLinked indicates a source is already associated with a document.
var ErrSourceAlreadyLinked = errors.New("source already linked to a document")

// Document is the in-memory representation of a documents row.
type Document struct {
	ID              string
	AssetID         string
	SourceID        string
	DocType         DocType
	ExtractedFields json.RawMessage
	RawExtraction   json.RawMessage
	CreatedAt       time.Time
}

// DocumentWithSource extends Document with joined source metadata.
type DocumentWithSource struct {
	Document
	SourceFilename   string
	SourceUploadedAt time.Time
}

// Repo stores and retrieves documents.
type Repo struct {
	pool *pgxpool.Pool
}

// New returns a Repo backed by the provided connection pool.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const selectColumns = `id, asset_id, source_id, doc_type, extracted_fields, raw_extraction, created_at`

// scanRow decodes the standard document column set from a pgx.Row.
func scanRow(row pgx.Row) (Document, error) {
	var d Document
	var docType string
	var ef, re string
	err := row.Scan(&d.ID, &d.AssetID, &d.SourceID, &docType, &ef, &re, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	d.DocType = DocType(docType)
	d.ExtractedFields = json.RawMessage(ef)
	d.RawExtraction = json.RawMessage(re)
	return d, nil
}

// Create inserts a new document and returns the inserted row.
// If DocType is invalid, ErrInvalidDocType is returned without database access.
// If the source is already linked to another document, ErrSourceAlreadyLinked is returned.
func (r *Repo) Create(ctx context.Context, d Document) (Document, error) {
	if !d.DocType.Valid() {
		return Document{}, fmt.Errorf("documents: create: %w", ErrInvalidDocType)
	}

	const q = `INSERT INTO documents (
		asset_id, source_id, doc_type, extracted_fields, raw_extraction
	) VALUES ($1,$2,$3,$4,$5)
	RETURNING ` + selectColumns

	doc, err := scanRow(r.pool.QueryRow(ctx, q,
		d.AssetID, d.SourceID, string(d.DocType),
		string(d.ExtractedFields), string(d.RawExtraction),
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Document{}, fmt.Errorf("documents: create: %w", ErrSourceAlreadyLinked)
		}
		return Document{}, fmt.Errorf("documents: create: %w", err)
	}
	return doc, nil
}

// GetByID retrieves a document by its primary key.
func (r *Repo) GetByID(ctx context.Context, id string) (Document, error) {
	q := `SELECT ` + selectColumns + ` FROM documents WHERE id = $1`
	d, err := scanRow(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Document{}, fmt.Errorf("documents: get by id: %w", ErrNotFound)
		}
		return Document{}, fmt.Errorf("documents: get by id: %w", err)
	}
	return d, nil
}

// ListByAsset returns all documents for the given asset, joined with source
// metadata, ordered by ingestion time (created_at, id).
func (r *Repo) ListByAsset(ctx context.Context, assetID string) ([]DocumentWithSource, error) {
	q := `SELECT d.id, d.asset_id, d.source_id, d.doc_type, d.extracted_fields, d.raw_extraction, d.created_at,
		s.filename, s.uploaded_at
		FROM documents d
		JOIN sources s ON s.id = d.source_id
		WHERE d.asset_id = $1
		ORDER BY d.created_at, d.id`

	rows, err := r.pool.Query(ctx, q, assetID)
	if err != nil {
		return nil, fmt.Errorf("documents: list by asset: %w", err)
	}
	defer rows.Close()

	out := make([]DocumentWithSource, 0)
	for rows.Next() {
		var dws DocumentWithSource
		var docType string
		var ef, re string
		if err := rows.Scan(
			&dws.ID, &dws.AssetID, &dws.SourceID, &docType, &ef, &re, &dws.CreatedAt,
			&dws.SourceFilename, &dws.SourceUploadedAt,
		); err != nil {
			return nil, fmt.Errorf("documents: list by asset: %w", err)
		}
		dws.DocType = DocType(docType)
		dws.ExtractedFields = json.RawMessage(ef)
		dws.RawExtraction = json.RawMessage(re)
		out = append(out, dws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("documents: list by asset: %w", err)
	}
	return out, nil
}
