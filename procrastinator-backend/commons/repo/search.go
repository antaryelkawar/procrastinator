package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

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
	SearchAssets(ctx context.Context, pattern string, opts ...Option) ([]entity.Asset, error)
	SearchAccounts(ctx context.Context, pattern string, opts ...Option) ([]entity.FinancialAccount, error)
	SearchMovements(ctx context.Context, pattern string, opts ...Option) ([]entity.MoneyMovement, error)
	SearchDocuments(ctx context.Context, pattern string, opts ...Option) ([]entity.Document, error)
	SearchImportBatches(ctx context.Context, pattern string, opts ...Option) ([]entity.ImportBatch, error)
}
