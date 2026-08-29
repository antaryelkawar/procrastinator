package postgres

import (
	"context"
	"fmt"
	"strings"

	"procrastinator-backend/commons/repo"
)

// householdsForUser returns the ids of every household the given user is a
// member of. Returns a non-nil empty slice when the user belongs to no
// household.
func householdsForUser(ctx context.Context, q Querier, userID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT household_id FROM household_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListScoped lists rows visible to the resolved tenant/user under the spec's
// scope access rule: the user's personal rows (scope_type = 'personal') plus
// rows owned by a household the user is a member of (owner_household_id IN
// the user's households). A user with no household memberships sees only
// personal rows. The tenant boundary itself is enforced by RLS; ListScoped
// adds the application-level scope filter on top.
func (r *pgRepository[T]) ListScoped(ctx context.Context, opts ...repo.Option) ([]T, error) {
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

	var result []T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		households, err := householdsForUser(ctx, q, tid)
		if err != nil {
			return err
		}

		var conds []string
		var args []any
		addCond := func(clause string, arg any) {
			args = append(args, arg)
			conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
		}
		addCond("tenant_id", tid)

		if len(households) == 0 {
			// No memberships: only personal rows are visible. Emitting
			// "owner_household_id IN ()" would be invalid SQL.
			conds = append(conds, "scope_type = 'personal'")
		} else {
			placeholders := make([]string, len(households))
			for i, id := range households {
				args = append(args, id)
				placeholders[i] = fmt.Sprintf("$%d", len(args))
			}
			conds = append(conds, fmt.Sprintf(
				"(scope_type = 'personal' OR owner_household_id IN (%s))",
				strings.Join(placeholders, ", ")))
		}
		for _, f := range o.Filters {
			addFilterCond(r.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT * FROM ")
		sb.WriteString(r.table)
		sb.WriteString(" WHERE ")
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

		result = make([]T, 0)
		for rows.Next() {
			item, err := r.scanRow(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}
