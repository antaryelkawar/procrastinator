package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"procrastinator-backend/commons-data"
)

// ErrSourceAlreadyLinked is returned by DocumentRepository.Create when the
// insert violates the UNIQUE constraint on documents.source_id, i.e. the
// source file has already been linked to a document.
var ErrSourceAlreadyLinked = errors.New("source already linked to a document")

var _ data.DocumentRepository = (*pgDocumentRepo)(nil)

// pgDocumentRepo implements data.DocumentRepository on top of a pgx Querier.
type pgDocumentRepo struct {
	q Querier
}

// NewDocumentRepo returns a document repository bound to q.
func NewDocumentRepo(q Querier) *pgDocumentRepo {
	return &pgDocumentRepo{q: q}
}

const documentColumns = `id, tenant_id, asset_id, source_id, doc_type,
	extracted_fields, raw_extraction, created_at`

// documentColumnsPrefixed is documentColumns with the d. table qualifier,
// for use in joins where the same column names exist on both sides.
const documentColumnsPrefixed = `d.id, d.tenant_id, d.asset_id, d.source_id, d.doc_type,
	d.extracted_fields, d.raw_extraction, d.created_at`

func (r *pgDocumentRepo) Create(ctx context.Context, d data.Document) (data.Document, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Document{}, err
	}
	if !data.ValidDocType(d.DocType) {
		return data.Document{}, fmt.Errorf("documents: invalid doc_type %q", d.DocType)
	}
	d.TenantID = tenant

	fields, err := toMetadataJSON(d.ExtractedFields)
	if err != nil {
		return data.Document{}, fmt.Errorf("encode extracted_fields: %w", err)
	}
	raw, err := json.Marshal(d.RawExtraction)
	if err != nil {
		return data.Document{}, fmt.Errorf("encode raw_extraction: %w", err)
	}

	const stmt = `
		INSERT INTO documents (
			id, tenant_id, asset_id, source_id, doc_type,
			extracted_fields, raw_extraction
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5::jsonb, $6::jsonb)
		RETURNING ` + documentColumns
	row := r.q.QueryRow(ctx, stmt,
		d.TenantID, d.AssetID, d.SourceID, d.DocType, fields, raw,
	)
	doc, err := scanDocument(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return data.Document{}, fmt.Errorf("documents: source %s: %w", d.SourceID, ErrSourceAlreadyLinked)
		}
		return data.Document{}, err
	}
	return doc, nil
}

func (r *pgDocumentRepo) GetByID(ctx context.Context, id string) (data.Document, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return data.Document{}, err
	}
	const stmt = `SELECT ` + documentColumns + ` FROM documents WHERE id = $1 AND tenant_id = $2`
	return scanDocument(r.q.QueryRow(ctx, stmt, id, tenant))
}

func (r *pgDocumentRepo) ListByAsset(ctx context.Context, assetID string) ([]data.DocumentWithSource, error) {
	tenant, err := data.TenantFrom(ctx)
	if err != nil {
		return nil, err
	}
	const stmt = `
		SELECT ` + documentColumnsPrefixed + `, s.filename, s.uploaded_at
		FROM documents d
		JOIN sources s ON s.id = d.source_id
		WHERE d.tenant_id = $1 AND d.asset_id = $2
		ORDER BY d.created_at, d.id`
	rows, err := r.q.Query(ctx, stmt, tenant, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := make([]data.DocumentWithSource, 0)
	for rows.Next() {
		var dw data.DocumentWithSource
		var id, filename string
		var fieldsJSON, rawJSON []byte
		err := rows.Scan(
			&id, &dw.TenantID, &dw.AssetID, &dw.SourceID, &dw.DocType,
			&fieldsJSON, &rawJSON, &dw.CreatedAt,
			&filename, &dw.SourceUploadedAt,
		)
		if err != nil {
			return nil, err
		}
		dw.ID = id
		dw.SourceFilename = filename
		if err := decodeDocumentJSON(fieldsJSON, rawJSON, &dw.ExtractedFields, &dw.RawExtraction); err != nil {
			return nil, err
		}
		docs = append(docs, dw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return docs, nil
}

// scanDocument scans a single document row from any pgx.Rows or pgx Row
// (both expose Scan with the same semantics). pgx.ErrNoRows is mapped to
// data.ErrNotFound so callers can use errors.Is uniformly.
func scanDocument(row rowScanner) (data.Document, error) {
	var d data.Document
	var id string
	var fieldsJSON, rawJSON []byte
	err := row.Scan(
		&id, &d.TenantID, &d.AssetID, &d.SourceID, &d.DocType,
		&fieldsJSON, &rawJSON, &d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return data.Document{}, data.ErrNotFound
		}
		return data.Document{}, err
	}
	d.ID = id
	if err := decodeDocumentJSON(fieldsJSON, rawJSON, &d.ExtractedFields, &d.RawExtraction); err != nil {
		return data.Document{}, err
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
