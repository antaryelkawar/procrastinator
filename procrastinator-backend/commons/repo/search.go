package repo

import (
	"context"
	"time"

	"procrastinator-backend/commons/entity"
)

// Filters carries structured search filters applied to SearchAssets and
// SearchDocuments. All fields are optional; nil means "no filter".
type Filters struct {
	// Category filters to assets with the given asset_category.
	Category *string
	// Brand filters to assets whose brand case-insensitively contains this substring.
	Brand *string
	// PurchaseFrom filters to assets purchased on or after this date (inclusive).
	PurchaseFrom *time.Time
	// PurchaseTo filters to assets purchased on or before this date (inclusive).
	PurchaseTo *time.Time
	// WarrantyStatus filters by computed warranty status. Accepted values:
	//   "active"            — warranty_end > now()
	//   "expiring_within:N" — now() < warranty_end <= now()+N days
	//   "expired"           — warranty_end < now()
	WarrantyStatus *string
	// HasDocuments filters to assets that have (true) or lack (false) linked documents.
	HasDocuments *bool
	// DocClassification filters to documents with the given doc_type.
	DocClassification *string
}

// SearchBackend provides tenant-scoped full-text search across the owned
// entity types. Each method runs a custom per-type ILIKE query in a
// scope-bound transaction (so the app.user_id RLS backstop applies), applies
// the D-8 app-level visibility rule, matches a case-insensitive literal
// substring via ILIKE, and returns the matching rows ordered by created_at
// DESC, id ASC.
//
// The pattern argument is expected to be pre-escaped by the caller: the search
// core service escapes the ILIKE metacharacters % / _ / \ so they match
// literally. Implementations must not re-escape the pattern.
type SearchBackend interface {
	// SearchAssets returns assets whose name, brand, model, or serial_number
	// match the pre-escaped ILIKE pattern, ANDed with any structured filters
	// (category, brand, date range, warranty status, has-documents).
	SearchAssets(ctx context.Context, pattern string, filters Filters, opts ...Option) ([]entity.Asset, error)
	SearchAccounts(ctx context.Context, pattern string, opts ...Option) ([]entity.FinancialAccount, error)
	SearchMovements(ctx context.Context, pattern string, opts ...Option) ([]entity.MoneyMovement, error)
	// SearchDocuments returns documents whose joined source filename matches
	// the pre-escaped ILIKE pattern, ANDed with any structured filters
	// (doc classification).
	SearchDocuments(ctx context.Context, pattern string, filters Filters, opts ...Option) ([]entity.Document, error)
	SearchImportBatches(ctx context.Context, pattern string, opts ...Option) ([]entity.ImportBatch, error)
}
