package postgres

import (
	"context"
	"fmt"
	"strings"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guard: DocumentRepository satisfies the generic repository
// interface for its entity.
var _ repo.Repository[entity.Document] = (*DocumentRepository)(nil)

// DocumentRepository is the generic repository engine for entity.Document,
// augmented with the document-specific aggregate queries below.
type DocumentRepository struct {
	*pgRepository[entity.Document]
}

// BalanceForAccount returns the derived balance of one account as an exact
// decimal string: money in (movements with the account as destination) minus
// money out (movements with the account as source), across all of the
// user's movements. Transfers count in both directions naturally. An
// account with no movements yields "0".
func (r *MovementRepository) BalanceForAccount(ctx context.Context, accountID string, opts ...repo.Option) (string, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return "", err
	}

	var balance string
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, accountID)
		accN := len(args)
		stmt := fmt.Sprintf(
			"SELECT COALESCE(SUM((payload->'data'->>'amount')::numeric) FILTER (WHERE destination_account_id = $%d), 0) - COALESCE(SUM((payload->'data'->>'amount')::numeric) FILTER (WHERE source_account_id = $%d), 0) FROM money_movements WHERE %s",
			accN, accN, vis)
		return q.QueryRow(ctx, stmt, args...).Scan(&balance)
	})
	return balance, err
}

// MovementsForAccount returns every movement of the user that touches
// accountID as source or destination. Caller-supplied filters, ordering,
// limit, and offset are validated against the movement whitelist and applied
// exactly like the generic List. Returns a non-nil empty slice when no rows.
func (r *MovementRepository) MovementsForAccount(ctx context.Context, accountID string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(r.codec.filters, o.Filters); err != nil {
		return nil, err
	}
	if err := validateOrderBy(r.codec.filters, o.OrderBy); err != nil {
		return nil, err
	}

	var result []entity.MoneyMovement
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		conds = append(conds, vis)
		conds = append(conds, fmt.Sprintf("(source_account_id = $%d OR destination_account_id = $%d)", len(args)+1, len(args)+2))
		args = append(args, accountID, accountID)
		for _, f := range o.Filters {
			addFilterCond(r.codec.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT ")
		sb.WriteString(r.selectList())
		sb.WriteString(" FROM money_movements WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
		if o.OrderBy != "" {
			sb.WriteString(" ORDER BY ")
			sb.WriteString(orderClause(r.codec.filters, o.OrderBy))
		}
		if o.Limit > 0 {
			args = append(args, o.Limit)
			sb.WriteString(fmt.Sprintf(" LIMIT $%d", len(args)))
		}
		if o.Offset > 0 {
			args = append(args, o.Offset)
			sb.WriteString(fmt.Sprintf(" OFFSET $%d", len(args)))
		}

		rows, err := q.Query(ctx, sb.String(), args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.MoneyMovement, 0)
		for rows.Next() {
			item, err := scanMoneyMovement(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// LinkCandidates returns the user's documents that could be auto-linked to a
// movement by exact amount+currency match in extracted_fields, excluding any
// document already linked to a same-user movement. Caller-supplied
// filters, ordering, limit, and offset are validated against the document
// whitelist and applied exactly like the generic List. Returns a non-nil
// empty slice.
func (r *DocumentRepository) LinkCandidates(ctx context.Context, amount, currency string, opts ...repo.Option) ([]entity.Document, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(r.codec.filters, o.Filters); err != nil {
		return nil, err
	}
	if err := validateOrderBy(r.codec.filters, o.OrderBy); err != nil {
		return nil, err
	}

	var result []entity.Document
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		addCond := func(clause string, arg any) {
			args = append(args, arg)
			conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
		}

		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		conds = append(conds, vis)
		// hhCount is the number of bind args consumed by the visibility
		// fragment: 1 (no households) or 1 + (#households). The NOT EXISTS
		// subquery below must reference exactly these placeholders ($1 = user,
		// $2..$hhCount = households) and append nothing of its own — so capture
		// it NOW, before the price/currency args are added.
		hhCount := len(args)
		addCond("(payload #>> '{data,extracted_fields,price}')", amount)
		addCond("(payload #>> '{data,extracted_fields,currency}')", currency)

		// The NOT EXISTS subquery is owner-scoped by default; it adds the
		// aliased household disjunct only when the user actually has household
		// memberships, reusing the same household placeholders as the main
		// visibility fragment (no new bind args).
		sub := "m.owner_id = $1"
		if hhCount > 1 {
			placeholders := make([]string, hhCount-1)
			for i := 0; i < hhCount-1; i++ {
				placeholders[i] = fmt.Sprintf("$%d", i+2)
			}
			sub = "(m.owner_id = $1 OR m.owner_household_id IN (" + strings.Join(placeholders, ", ") + "))"
		}
		conds = append(conds, fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM money_movements m WHERE %s AND m.linked_document_id = documents.id)", sub))
		for _, f := range o.Filters {
			addFilterCond(r.codec.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT ")
		sb.WriteString(r.selectList())
		sb.WriteString(" FROM documents WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
		if o.OrderBy != "" {
			sb.WriteString(" ORDER BY ")
			sb.WriteString(orderClause(r.codec.filters, o.OrderBy))
		}
		if o.Limit > 0 {
			args = append(args, o.Limit)
			sb.WriteString(fmt.Sprintf(" LIMIT $%d", len(args)))
		}
		if o.Offset > 0 {
			args = append(args, o.Offset)
			sb.WriteString(fmt.Sprintf(" OFFSET $%d", len(args)))
		}

		rows, err := q.Query(ctx, sb.String(), args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.Document, 0)
		for rows.Next() {
			item, err := scanDocument(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// SearchDocuments returns the user's documents whose joined source filename
// case-insensitively contains the (pre-escaped) ILIKE pattern, ANDed with any
// structured filters (doc classification), ordered by created_at DESC, id ASC.
// The join is on documents.source_id = sources.id; the D-8 visibility predicate
// is applied on the documents row (aliased "d") and relies on the same-scope
// invariant that a document and its source share owner_id / owner_household_id.
// It runs in a scope-bound transaction (app.user_id RLS backstop) and returns a
// non-nil empty slice when nothing matches.
func (r *DocumentRepository) SearchDocuments(ctx context.Context, pattern string, filters repo.Filters, opts ...repo.Option) ([]entity.Document, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.Document
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "d", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)
		extra := ""
		if filters.DocClassification != nil {
			args = append(args, *filters.DocClassification)
			extra = fmt.Sprintf(" AND (d.payload #>> '{data,doc_type}') = $%d", len(args))
		}
		prefixed := "SELECT d." + strings.Join(r.codec.selectCols, ", d.")
		stmt := fmt.Sprintf(
			"%s FROM documents d INNER JOIN sources s ON s.id = d.source_id WHERE %s AND (s.payload #>> '{data,filename}') ILIKE $%d%s ORDER BY d.created_at DESC, d.id ASC",
			prefixed, vis, patN, extra)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.Document, 0)
		for rows.Next() {
			item, err := scanDocument(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}
