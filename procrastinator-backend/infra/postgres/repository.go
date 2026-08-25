package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"procrastinator-backend/commons/repo"
)

var (
	_ repo.Repository[struct{}] = (*pgRepository[struct{}])(nil)
)

// pgRepository is a generic repository engine that implements repo.Repository[T].
// It builds SQL from the table name and a map of column→value pairs supplied
// by the entity-specific toMap function.
type pgRepository[T any] struct {
	q       Querier
	table   string
	scanRow func(rowScanner) (T, error)
	toMap   func(T) map[string]any
}

// Get retrieves a single row by ID, optionally scoped by tenant and filtered.
func (r *pgRepository[T]) Get(ctx context.Context, id string, opts ...repo.Option) (T, error) {
	o := repo.ApplyOptions(opts...)

	var conds []string
	var args []any
	addCond := func(clause string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
	}

	args = append(args, id)
	conds = append(conds, "id = $1")

	if o.TenantID != "" {
		addCond("tenant_id", o.TenantID)
	}
	for _, f := range o.Filters {
		args = append(args, f.Value)
		conds = append(conds, fmt.Sprintf("%s %s $%d", f.Field, f.Op, len(args)))
	}

	stmt := fmt.Sprintf("SELECT * FROM %s WHERE %s", r.table, strings.Join(conds, " AND "))
	return r.scanRow(r.q.QueryRow(ctx, stmt, args...))
}

// List retrieves multiple rows with optional tenant scoping, filters, ordering, and pagination.
func (r *pgRepository[T]) List(ctx context.Context, opts ...repo.Option) ([]T, error) {
	o := repo.ApplyOptions(opts...)

	var conds []string
	var args []any
	addCond := func(clause string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf("%s = $%d", clause, len(args)))
	}

	if o.TenantID != "" {
		addCond("tenant_id", o.TenantID)
	}
	for _, f := range o.Filters {
		args = append(args, f.Value)
		conds = append(conds, fmt.Sprintf("%s %s $%d", f.Field, f.Op, len(args)))
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
		sb.WriteString(o.OrderBy)
	}
	if o.Limit > 0 {
		args = append(args, o.Limit)
		sb.WriteString(fmt.Sprintf(" LIMIT $%d", len(args)))
	}
	if o.Offset > 0 {
		args = append(args, o.Offset)
		sb.WriteString(fmt.Sprintf(" OFFSET $%d", len(args)))
	}

	rows, err := r.q.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]T, 0)
	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Create inserts a new entity. The DB generates the id via gen_random_uuid().
func (r *pgRepository[T]) Create(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	o := repo.ApplyOptions(opts...)

	m := r.toMap(ent)
	if o.TenantID != "" {
		m["tenant_id"] = o.TenantID
	}
	delete(m, "id")

	var cols []string
	var vals []string
	var args []any
	// First column is always id with gen_random_uuid()
	cols = append(cols, "id")
	vals = append(vals, "gen_random_uuid()")
	for col, val := range m {
		args = append(args, val)
		cols = append(cols, col)
		vals = append(vals, fmt.Sprintf("$%d", len(args)))
	}

	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		r.table, strings.Join(cols, ", "), strings.Join(vals, ", "))
	return r.scanRow(r.q.QueryRow(ctx, stmt, args...))
}

// Update modifies an existing entity. Only non-zero fields in the toMap result
// are SET. If no fields to update, it simply re-fetches the entity.
func (r *pgRepository[T]) Update(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

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
	if o.TenantID != "" {
		whereArgs = append(whereArgs, o.TenantID)
		whereClause += fmt.Sprintf(" AND tenant_id = $%d", len(whereArgs))
	}

	stmt := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING *",
		r.table, strings.Join(setClauses, ", "), whereClause)
	return r.scanRow(r.q.QueryRow(ctx, stmt, whereArgs...))
}

// Delete removes a row by ID, optionally scoped by tenant.
func (r *pgRepository[T]) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	o := repo.ApplyOptions(opts...)

	var conds []string
	var args []any
	args = append(args, id)
	conds = append(conds, "id = $1")
	if o.TenantID != "" {
		args = append(args, o.TenantID)
		conds = append(conds, fmt.Sprintf("tenant_id = $%d", len(args)))
	}

	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", r.table, strings.Join(conds, " AND "))
	_, err := r.q.Exec(ctx, stmt, args...)
	return err
}
