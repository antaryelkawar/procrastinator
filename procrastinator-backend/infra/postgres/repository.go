package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
)

var (
	_ repo.Repository[struct{}] = (*pgRepository[struct{}])(nil)
)

// pgRepository is a generic repository engine that implements repo.Repository[T].
// It builds SQL from the table name and a map of column→value pairs supplied
// by the entity-specific toMap function.
type pgRepository[T any] struct {
	scope   txScope
	table   string
	scanRow func(rowScanner) (T, error)
	toMap   func(T) map[string]any
	filters filterConfig
}

// validOps is the fixed set of operators accepted in filter conditions.
var validOps = map[string]bool{
	"=":    true,
	"!=":   true,
	"<":    true,
	"<=":   true,
	">":    true,
	">=":   true,
	"LIKE": true,
	"IN":   true,
}

// validateFilters rejects any filter whose field or operator is not in the
// entity whitelist, and any IN value that is not a slice.
func validateFilters(cfg filterConfig, filters []repo.Filter) error {
	for _, f := range filters {
		if _, ok := cfg.fieldCols[f.Field]; !ok {
			return fmt.Errorf("postgres: unknown filter field %q", f.Field)
		}
		if !validOps[f.Op] {
			return fmt.Errorf("postgres: unknown operator %q", f.Op)
		}
		if f.Op == "IN" {
			v := reflect.ValueOf(f.Value)
			if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
				return fmt.Errorf("postgres: IN operator requires a slice value, got %T", f.Value)
			}
		}
	}
	return nil
}

// validateOrderBy rejects any comma-separated OrderBy token that is not a key
// in the entity's order whitelist.
func validateOrderBy(cfg filterConfig, orderBy string) error {
	if orderBy == "" {
		return nil
	}
	for _, p := range strings.Split(orderBy, ",") {
		token := strings.TrimSpace(p)
		if _, ok := cfg.orderCols[token]; !ok {
			return fmt.Errorf("postgres: unknown order column %q", token)
		}
	}
	return nil
}

// addFilterCond appends a whitelisted filter condition to conds, expanding IN
// to per-element bind placeholders. The caller must have validated the filter.
func addFilterCond(cfg filterConfig, conds *[]string, args *[]any, f repo.Filter) {
	col := cfg.fieldCols[f.Field]
	if f.Op == "IN" {
		slice := reflect.ValueOf(f.Value)
		n := slice.Len()
		if n == 0 {
			// IN () is invalid SQL; FALSE matches nothing.
			*conds = append(*conds, "FALSE")
			return
		}
		placeholders := make([]string, n)
		for i := 0; i < n; i++ {
			*args = append(*args, slice.Index(i).Interface())
			placeholders[i] = fmt.Sprintf("$%d", len(*args))
		}
		*conds = append(*conds, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
		return
	}
	*args = append(*args, f.Value)
	*conds = append(*conds, fmt.Sprintf("%s %s $%d", col, f.Op, len(*args)))
}

// orderClause maps each comma-separated OrderBy token through the entity's
// order whitelist. The caller must have validated the tokens.
func orderClause(cfg filterConfig, orderBy string) string {
	parts := strings.Split(orderBy, ",")
	mapped := make([]string, len(parts))
	for i, p := range parts {
		mapped[i] = cfg.orderCols[strings.TrimSpace(p)]
	}
	return strings.Join(mapped, ", ")
}

// resolveTenant resolves the tenant ID from the option (preferred) or context.
// Returns tenant.ErrNoTenant if neither is available.
func resolveTenant(ctx context.Context, o *repo.Options) (string, error) {
	if o.TenantID != "" {
		return o.TenantID, nil
	}
	return tenant.TenantFrom(ctx)
}

// Get retrieves a single row by ID, scoped by tenant and optionally filtered.
func (r *pgRepository[T]) Get(ctx context.Context, id string, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return zero, err
	}

	if err := validateFilters(r.filters, o.Filters); err != nil {
		return zero, err
	}

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		addCond := func(clause string, arg any) {
			args = append(args, arg)
			conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
		}
		args = append(args, id)
		conds = append(conds, "id = $1")
		addCond("tenant_id", tid)
		for _, f := range o.Filters {
			addFilterCond(r.filters, &conds, &args, f)
		}
		stmt := fmt.Sprintf("SELECT * FROM %s WHERE %s", r.table, strings.Join(conds, " AND "))
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, args...))
		return scanErr
	})
	return result, err
}

// List retrieves multiple rows scoped by tenant, with optional filters, ordering, and pagination.
func (r *pgRepository[T]) List(ctx context.Context, opts ...repo.Option) ([]T, error) {
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
		var conds []string
		var args []any
		addCond := func(clause string, arg any) {
			args = append(args, arg)
			conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
		}
		addCond("tenant_id", tid)
		for _, f := range o.Filters {
			addFilterCond(r.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT * FROM ")
		sb.WriteString(r.table)
		if len(conds) > 0 {
			sb.WriteString(" WHERE ")
			sb.WriteString(strings.Join(conds, " AND "))
		}
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

// Create inserts a new entity stamped with the resolved tenant.
// The DB generates the id via gen_random_uuid().
func (r *pgRepository[T]) Create(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return zero, err
	}

	m := r.toMap(ent)
	m["tenant_id"] = tid
	delete(m, "id")

	var cols []string
	var vals []string
	var args []any
	cols = append(cols, "id")
	vals = append(vals, "gen_random_uuid()")
	for col, val := range m {
		args = append(args, val)
		cols = append(cols, col)
		vals = append(vals, fmt.Sprintf("$%d", len(args)))
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		r.table, strings.Join(cols, ", "), strings.Join(vals, ", "))

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, args...))
		return scanErr
	})
	return result, err
}

// Update modifies an existing entity. Only non-zero fields in the toMap result
// are SET. If no fields to update, it simply re-fetches the entity.
func (r *pgRepository[T]) Update(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return zero, err
	}

	m := r.toMap(ent)
	idVal, ok := m["id"]
	if !ok {
		return zero, errors.New("postgres: update: entity has no id")
	}
	delete(m, "id")

	if len(m) == 0 {
		return r.Get(ctx, idVal.(string), opts...)
	}

	var setClauses []string
	var args []any
	for col, val := range m {
		args = append(args, val)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	whereArgs := append(args, idVal)
	whereClause := fmt.Sprintf("id = $%d", len(whereArgs))
	whereArgs = append(whereArgs, tid)
	whereClause += fmt.Sprintf(" AND tenant_id = $%d", len(whereArgs))

	stmt := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING *",
		r.table, strings.Join(setClauses, ", "), whereClause)

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, whereArgs...))
		return scanErr
	})
	return result, err
}

// Delete removes a row by ID, scoped by tenant.
func (r *pgRepository[T]) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveTenant(ctx, o)
	if err != nil {
		return err
	}

	var conds []string
	var args []any
	args = append(args, id)
	conds = append(conds, "id = $1")
	args = append(args, tid)
	conds = append(conds, fmt.Sprintf("tenant_id = $%d", len(args)))

	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", r.table, strings.Join(conds, " AND "))

	return r.scope.run(ctx, tid, func(q Querier) error {
		_, err := q.Exec(ctx, stmt, args...)
		return err
	})
}
