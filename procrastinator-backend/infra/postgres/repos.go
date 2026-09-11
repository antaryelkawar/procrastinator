package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guards: the concrete asset/source repositories satisfy the
// generic repository interface for their entity.
var (
	_ repo.Repository[entity.Asset]  = (*AssetRepository)(nil)
	_ repo.Repository[entity.Source] = (*SourceRepository)(nil)
)

// AssetRepository is the generic repository engine for entity.Asset, carrying
// the scope-aware (owner + household membership) visibility behavior.
type AssetRepository struct {
	*pgRepository[entity.Asset]
}

// SearchAssets returns the user's assets whose name, brand, model, or
// serial case-insensitively contain the (pre-escaped) ILIKE pattern,
// ANDed with any structured filters (category, brand substring, purchase date
// range, warranty status, has-documents), ordered by created_at DESC, id ASC.
// It runs in a scope-bound transaction (app.user_id RLS backstop), applies the
// D-8 visibility rule, and returns a non-nil empty slice when nothing matches.
func (r *AssetRepository) SearchAssets(ctx context.Context, pattern string, filters repo.Filters, opts ...repo.Option) ([]entity.Asset, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.Asset
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)

		var conds []string
		if filters.Category != nil {
			args = append(args, *filters.Category)
			conds = append(conds, fmt.Sprintf("(payload #>> '{data,asset_category}') = $%d", len(args)))
		}
		if filters.Brand != nil {
			args = append(args, "%"+*filters.Brand+"%")
			conds = append(conds, fmt.Sprintf("(payload #>> '{data,brand}') ILIKE $%d", len(args)))
		}
		if filters.PurchaseFrom != nil {
			args = append(args, *filters.PurchaseFrom)
			conds = append(conds, fmt.Sprintf("((payload #>> '{data,purchase_date}')::date) >= $%d", len(args)))
		}
		if filters.PurchaseTo != nil {
			args = append(args, *filters.PurchaseTo)
			conds = append(conds, fmt.Sprintf("((payload #>> '{data,purchase_date}')::date) <= $%d", len(args)))
		}
		if filters.WarrantyStatus != nil {
			switch {
			case *filters.WarrantyStatus == "active":
				conds = append(conds, "((payload #>> '{data,warranty_end}')::date) > now()")
			case *filters.WarrantyStatus == "expired":
				conds = append(conds, "((payload #>> '{data,warranty_end}')::date) < now()")
			case strings.HasPrefix(*filters.WarrantyStatus, "expiring_within:"):
				if days, err := strconv.Atoi(strings.TrimPrefix(*filters.WarrantyStatus, "expiring_within:")); err == nil {
					args = append(args, days)
					conds = append(conds, fmt.Sprintf("((payload #>> '{data,warranty_end}')::date) > now() AND ((payload #>> '{data,warranty_end}')::date) <= now() + ($%d * interval '1 day')", len(args)))
				}
			}
		}
		if filters.HasDocuments != nil {
			if *filters.HasDocuments {
				conds = append(conds, "EXISTS (SELECT 1 FROM documents WHERE documents.asset_id = assets.id)")
			} else {
				conds = append(conds, "NOT EXISTS (SELECT 1 FROM documents WHERE documents.asset_id = assets.id)")
			}
		}

		extra := ""
		if len(conds) > 0 {
			extra = " AND " + strings.Join(conds, " AND ")
		}
		stmt := fmt.Sprintf(
			"SELECT %s FROM assets WHERE %s AND ((payload #>> '{data,name}') ILIKE $%d OR (payload #>> '{data,brand}') ILIKE $%d OR (payload #>> '{data,model}') ILIKE $%d OR (payload #>> '{data,serial}') ILIKE $%d)%s ORDER BY created_at DESC, id ASC",
			r.selectList(), vis, patN, patN, patN, patN, extra)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.Asset, 0)
		for rows.Next() {
			item, err := scanAsset(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// SourceRepository is the generic repository engine for entity.Source,
// carrying the scope-aware (owner + household membership) visibility behavior.
type SourceRepository struct {
	*pgRepository[entity.Source]
}

// NewAssetRepository returns a repository for entity.Asset.
func NewAssetRepository(pool *pgxpool.Pool) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:     &poolScope{pool: pool},
			table:     "assets",
			scanRow:   scanAsset,
			codec:     assetCodec,
			shareable: true,
		},
	}
}

// NewSourceRepository returns a repository for entity.Source.
func NewSourceRepository(pool *pgxpool.Pool) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:     &poolScope{pool: pool},
			table:     "sources",
			scanRow:   scanSource,
			codec:     sourceCodec,
			shareable: true,
		},
	}
}

// NewDocumentRepository returns a repository for entity.Document, carrying the
// document-specific aggregate queries (e.g. LinkCandidates).
func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:     &poolScope{pool: pool},
			table:     "documents",
			scanRow:   scanDocument,
			codec:     documentCodec,
			shareable: true,
		},
	}
}

// newAssetRepoForTx returns an asset repository bound to an ambient transaction.
func newAssetRepoForTx(tx pgx.Tx) *AssetRepository {
	return &AssetRepository{
		pgRepository: &pgRepository[entity.Asset]{
			scope:     &txScopeImpl{tx: tx},
			table:     "assets",
			scanRow:   scanAsset,
			codec:     assetCodec,
			shareable: true,
		},
	}
}

// newSourceRepoForTx returns a source repository bound to an ambient transaction.
func newSourceRepoForTx(tx pgx.Tx) *SourceRepository {
	return &SourceRepository{
		pgRepository: &pgRepository[entity.Source]{
			scope:     &txScopeImpl{tx: tx},
			table:     "sources",
			scanRow:   scanSource,
			codec:     sourceCodec,
			shareable: true,
		},
	}
}

// newDocumentRepoForTx returns a document repository bound to an ambient transaction.
func newDocumentRepoForTx(tx pgx.Tx) *DocumentRepository {
	return &DocumentRepository{
		pgRepository: &pgRepository[entity.Document]{
			scope:     &txScopeImpl{tx: tx},
			table:     "documents",
			scanRow:   scanDocument,
			codec:     documentCodec,
			shareable: true,
		},
	}
}

// filterConfig holds the per-entity whitelists for dynamic query names:
// fieldCols maps filterable field names to SQL expressions for WHERE clauses
// (bare column names or payload data expressions), orderCols maps orderable
// field names to SQL expressions for ORDER BY clauses. Every
// caller-influenced name is validated against these maps before SQL assembly;
// unknown names are rejected, never interpolated.
type filterConfig struct {
	fieldCols map[string]string
	orderCols map[string]string
}
