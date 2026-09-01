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
// tenant's movements. Transfers count in both directions naturally. An
// account with no movements yields "0".
func (r *MovementRepository) BalanceForAccount(ctx context.Context, accountID string, opts ...repo.Option) (string, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return "", err
	}

	const stmt = `SELECT COALESCE(SUM(amount) FILTER (WHERE destination_account_id = $2), 0)
		- COALESCE(SUM(amount) FILTER (WHERE source_account_id = $2), 0)
	FROM money_movements WHERE tenant_id = $1`
	var balance string
	err = r.scope.run(ctx, tid, func(q Querier) error {
		return q.QueryRow(ctx, stmt, tid, accountID).Scan(&balance)
	})
	return balance, err
}

// MovementsForAccount returns every movement of the tenant that touches
// accountID as source or destination. Caller-supplied filters, ordering,
// limit, and offset are validated against the movement whitelist and applied
// exactly like the generic List. Returns a non-nil empty slice when no rows.
func (r *MovementRepository) MovementsForAccount(ctx context.Context, accountID string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(r.filters, o.Filters); err != nil {
		return nil, err
	}
	if err := validateOrderBy(r.filters, o.OrderBy); err != nil {
		return nil, err
	}

	var result []entity.MoneyMovement
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		addCond := func(clause string, arg any) {
			args = append(args, arg)
			conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
		}

		addCond("tenant_id", tid)
		conds = append(conds, fmt.Sprintf("(source_account_id = $%d OR destination_account_id = $%d)", len(args)+1, len(args)+2))
		args = append(args, accountID, accountID)
		for _, f := range o.Filters {
			addFilterCond(r.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT * FROM money_movements WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
		if o.OrderBy != "" {
			sb.WriteString(" ORDER BY ")
			sb.WriteString(orderClause(r.filters, o.OrderBy))
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

// LinkCandidates returns the tenant's documents that could be auto-linked to a
// movement by exact amount+currency match in extracted_fields, excluding any
// document already linked to a same-tenant movement. Caller-supplied
// filters, ordering, limit, and offset are validated against the document
// whitelist and applied exactly like the generic List. Returns a non-nil
// empty slice.
func (r *DocumentRepository) LinkCandidates(ctx context.Context, amount, currency string, opts ...repo.Option) ([]entity.Document, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return nil, err
	}
	if err := validateFilters(r.filters, o.Filters); err != nil {
		return nil, err
	}
	if err := validateOrderBy(r.filters, o.OrderBy); err != nil {
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

		addCond("tenant_id", tid)
		addCond("extracted_fields ->> 'price'", amount)
		addCond("extracted_fields ->> 'currency'", currency)
		conds = append(conds, fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM money_movements m WHERE m.tenant_id = $%d AND m.linked_document_id = documents.id)", len(args)+1))
		args = append(args, tid)
		for _, f := range o.Filters {
			addFilterCond(r.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT * FROM documents WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
		if o.OrderBy != "" {
			sb.WriteString(" ORDER BY ")
			sb.WriteString(orderClause(r.filters, o.OrderBy))
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
