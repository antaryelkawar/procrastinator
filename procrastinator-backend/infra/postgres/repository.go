package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

var (
	_ repo.Repository[struct{}] = (*pgRepository[struct{}])(nil)
)

// pgRepository is a generic repository engine that implements repo.Repository[T]
// on the uniform table shape: real key/FK/lifecycle columns plus a payload
// jsonb column carrying the kind's data at payload.data.*. The codec supplies
// the SELECT column list, the non-payload INSERT/UPDATE values, and the
// payload data object (with clearing keys for kinds that support it).
type pgRepository[T any] struct {
	scope   txScope
	table   string
	scanRow func(rowScanner) (T, error)
	codec   *payloadCodec[T]
	// shareable enables the household disjunct in the visibility predicate for
	// Get/List/Update. It is true only for tables that have an
	// owner_household_id column. Zero value false means owner-only scoping.
	shareable bool
}

// selectList returns the codec's explicit SELECT column list (also the scan
// order).
func (r *pgRepository[T]) selectList() string {
	return strings.Join(r.codec.selectCols, ", ")
}

// entityIDer is implemented by every entity kind: it exposes the row identity.
type entityIDer interface {
	GetID() string
}

// validOps is the fixed set of operators accepted in filter conditions.
var validOps = map[string]bool{
	"=":       true,
	"!=":      true,
	"<":       true,
	"<=":      true,
	">":       true,
	">=":      true,
	"LIKE":    true,
	"IN":      true,
	"IS NULL": true,
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
// The column expressions come from the entity whitelist (trusted constants,
// never caller input).
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
	if f.Op == "IS NULL" {
		*conds = append(*conds, col+" IS NULL")
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

// resolveOwner resolves the user ID from the option (preferred) or context.
// Returns user.ErrNoUser if neither is available.
func resolveOwner(ctx context.Context, o *repo.Options) (string, error) {
	if o.OwnerID != "" {
		return o.OwnerID, nil
	}
	return user.UserFrom(ctx)
}

// Get retrieves a single row by ID, scoped by user and optionally filtered.
func (r *pgRepository[T]) Get(ctx context.Context, id string, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return zero, err
	}

	if err := validateFilters(r.codec.filters, o.Filters); err != nil {
		return zero, err
	}

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		args = append(args, id)
		conds = append(conds, "id = $1")
		vis, err := visibilityCond(ctx, q, tid, r.shareable, "", &args)
		if err != nil {
			return err
		}
		conds = append(conds, vis)
		for _, f := range o.Filters {
			addFilterCond(r.codec.filters, &conds, &args, f)
		}
		stmt := fmt.Sprintf("SELECT %s FROM %s WHERE %s", r.selectList(), r.table, strings.Join(conds, " AND "))
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, args...))
		return scanErr
	})
	return result, err
}

// List retrieves multiple rows scoped by user, with optional filters, ordering, and pagination.
func (r *pgRepository[T]) List(ctx context.Context, opts ...repo.Option) ([]T, error) {
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

	var result []T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var conds []string
		var args []any
		vis, err := visibilityCond(ctx, q, tid, r.shareable, "", &args)
		if err != nil {
			return err
		}
		conds = append(conds, vis)
		for _, f := range o.Filters {
			addFilterCond(r.codec.filters, &conds, &args, f)
		}

		var sb strings.Builder
		sb.WriteString("SELECT ")
		sb.WriteString(r.selectList())
		sb.WriteString(" FROM ")
		sb.WriteString(r.table)
		if len(conds) > 0 {
			sb.WriteString(" WHERE ")
			sb.WriteString(strings.Join(conds, " AND "))
		}
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

// Create inserts a new entity stamped with the resolved user.
// The DB generates the id via gen_random_uuid(). The payload column is
// included only when the codec's data map is non-empty (the clear list is
// ignored on Create).
func (r *pgRepository[T]) Create(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return zero, err
	}

	m := r.codec.columns(ent)
	data, _ := r.codec.marshal(ent)

	var cols []string
	var vals []string
	var args []any
	cols = append(cols, "id", "owner_id")
	vals = append(vals, "gen_random_uuid()", "$1")
	args = append(args, tid)
	for col, val := range m {
		args = append(args, val)
		cols = append(cols, col)
		vals = append(vals, fmt.Sprintf("$%d", len(args)))
	}
	if len(data) > 0 {
		payload, err := encodePayload(data)
		if err != nil {
			return zero, err
		}
		args = append(args, payload)
		cols = append(cols, "payload")
		vals = append(vals, fmt.Sprintf("$%d", len(args)))
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		r.table, strings.Join(cols, ", "), strings.Join(vals, ", "), r.selectList())

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		// A household-scoped row may only be created by a member of that
		// household. For tables without an owner_household_id column (e.g.
		// households) m has no such key, so this is a no-op.
		if hid := strFromAny(m["owner_household_id"]); hid != "" {
			member, err := isMember(ctx, q, tid, hid)
			if err != nil {
				return err
			}
			if !member {
				return repo.ErrNotMember
			}
		}
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, args...))
		return scanErr
	})
	return result, err
}

// isBareColumn reports whether a whitelisted SQL expression is a bare column
// name (no parentheses) as opposed to a payload expression like
// (payload #>> '{data,KEY}') or ((payload #>> '{data,KEY}')::date).
func isBareColumn(expr string) bool {
	return !strings.Contains(expr, "(")
}

// dataKeyOf extracts the data.* key from a payload expression of the form
// (payload #>> '{data,KEY}') or ((payload #>> '{data,KEY}')::<cast>). It is
// only called for non-bare expressions from the trusted whitelist maps.
func dataKeyOf(expr string) string {
	const open = "'{data,"
	const close = "}'"
	s := strings.Index(expr, open)
	if s < 0 {
		return ""
	}
	rest := expr[s+len(open):]
	e := strings.Index(rest, close)
	if e < 0 {
		return ""
	}
	return rest[:e]
}

// Update modifies an existing entity with payload merge semantics:
//   - real columns are SET from the codec's column values (nil pointers map to
//     SQL NULL, as in the old engine's toMap result);
//   - the payload's data object is merged: the stored data object is fetched
//     first, the codec's clear list removes keys, then the codec's data map
//     and UpdateSets data upserts overwrite/insert keys;
//   - UpdateSets on real columns SET the column (nil clears it to SQL NULL,
//     overriding the codec value); UpdateSets on data keys upsert (nil deletes
//     the key from the merged data object).
//
// If there are no column SETs, no data upserts, no deletes, and no UpdateSets,
// the entity is simply re-fetched (the old no-op short-circuit).
func (r *pgRepository[T]) Update(ctx context.Context, ent T, opts ...repo.Option) (T, error) {
	var zero T
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return zero, err
	}

	ider, ok := any(ent).(entityIDer)
	if !ok {
		return zero, errors.New("postgres: update: entity has no id")
	}
	id := ider.GetID()
	if id == "" {
		return zero, errors.New("postgres: update: entity has no id")
	}

	// 1. Real-column SETs (nil values bind as SQL NULL). An UpdateSet that
	// targets the same real column supersedes the codec's own value for it
	// (step 3), so we skip the codec assignment here to avoid emitting the
	// column twice (SQLSTATE 42601 "multiple assignments to same column").
	updatedCols := make(map[string]bool)
	for field := range o.UpdateSets {
		if expr, ok := r.codec.filters.fieldCols[field]; ok && isBareColumn(expr) {
			updatedCols[expr] = true
		}
	}
	setClauses := make([]string, 0, len(r.codec.columns(ent)))
	var args []any
	for col, val := range r.codec.columns(ent) {
		if updatedCols[col] {
			continue
		}
		args = append(args, val)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	// 2. Codec data upserts + clearing keys.
	data, clear := r.codec.marshal(ent)
	if data == nil {
		data = map[string]any{}
	}
	upserts := make(map[string]any, len(data))
	for k, v := range data {
		upserts[k] = v
	}
	clearSet := make(map[string]bool, len(clear))
	for _, k := range clear {
		clearSet[k] = true
	}

	// 3. UpdateSets: field → value, nil = clear. Whitelist-gated; a bare
	// column expression is a real column (nil → SQL NULL), anything else is a
	// data key (nil → delete from the merged data object).
	dataUpserts := make(map[string]any)
	dataDeletes := make(map[string]bool)
	for field, val := range o.UpdateSets {
		expr, ok := r.codec.filters.fieldCols[field]
		if !ok {
			return zero, fmt.Errorf("postgres: unknown update field %q", field)
		}
		if isBareColumn(expr) {
			if val == nil {
				// A `= NULL` literal needs no bound parameter. Appending nil to
				// args would shift every later placeholder and leave an untyped
				// $N (SQLSTATE 42P18: "could not determine data type of $N").
				setClauses = append(setClauses, expr+" = NULL")
			} else {
				args = append(args, val)
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", expr, len(args)))
			}
		} else if val == nil {
			dataDeletes[dataKeyOf(expr)] = true
		} else {
			dataUpserts[dataKeyOf(expr)] = val
		}
	}

	// 4. No-op short-circuit (preserved from the old engine): nothing to SET
	// and nothing to merge → re-fetch. The codec's own data upserts count as
	// work: a codec-driven update (e.g. setting a data field on an entity that
	// has no UpdateSets) must not be treated as a no-op.
	if len(setClauses) == 0 && len(upserts) == 0 && len(dataUpserts) == 0 && len(dataDeletes) == 0 &&
		len(clearSet) == 0 && len(o.UpdateSets) == 0 {
		return r.Get(ctx, id, opts...)
	}

	// 5. Fetch the existing payload FIRST and merge in Go: start from the
	// stored data object (empty when absent), apply the delete lists, then the
	// data upserts (codec + UpdateSets).
	merged, err := r.fetchExistingData(ctx, tid, id)
	if err != nil {
		return zero, err
	}
	for k := range clearSet {
		delete(merged, k)
	}
	for k := range dataDeletes {
		delete(merged, k)
	}
	for k, v := range upserts {
		merged[k] = v
	}
	for k, v := range dataUpserts {
		merged[k] = v
	}

	// The payload is rewritten whenever any merge input was applied: a
	// codec-driven data upsert, an UpdateSets upsert/delete, or a clear key.
	needsPayload := len(upserts) > 0 || len(dataUpserts) > 0 || len(dataDeletes) > 0 || len(clearSet) > 0
	if len(setClauses) == 0 && !needsPayload {
		// Defensive: unreachable after the short-circuit above, but the UPDATE
		// would be empty.
		return r.Get(ctx, id, opts...)
	}
	if needsPayload {
		payloadBytes, err := encodePayload(merged)
		if err != nil {
			return zero, err
		}
		args = append(args, payloadBytes)
		setClauses = append(setClauses, fmt.Sprintf("payload = $%d", len(args)))
	}

	whereArgs := append([]any(nil), args...)
	whereArgs = append(whereArgs, id)
	idPos := len(whereArgs)

	var result T
	err = r.scope.run(ctx, tid, func(q Querier) error {
		vis, err := visibilityCond(ctx, q, tid, r.shareable, "", &whereArgs)
		if err != nil {
			return err
		}
		whereClause := fmt.Sprintf("id = $%d AND %s", idPos, vis)
		stmt := fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING %s",
			r.table, strings.Join(setClauses, ", "), whereClause, r.selectList())
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, whereArgs...))
		return scanErr
	})
	return result, err
}

// fetchExistingData returns the stored data object of the row (never nil;
// empty map when the payload is absent or has no data). It maps
// pgx.ErrNoRows to repo.ErrNotFound.
func (r *pgRepository[T]) fetchExistingData(ctx context.Context, tid, id string) (map[string]any, error) {
	var existing []byte
	err := r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		args = append(args, id)
		vis, err := visibilityCond(ctx, q, tid, r.shareable, "", &args)
		if err != nil {
			return err
		}
		stmt := fmt.Sprintf("SELECT payload FROM %s WHERE id = $1 AND %s", r.table, vis)
		if err := q.QueryRow(ctx, stmt, args...).Scan(&existing); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return repo.ErrNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	data, err := decodePayloadData(existing)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

// Delete removes a row by ID, scoped by user.
func (r *pgRepository[T]) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return err
	}

	var conds []string
	var args []any
	args = append(args, id)
	conds = append(conds, "id = $1")
	args = append(args, tid)
	conds = append(conds, fmt.Sprintf("owner_id = $%d", len(args)))

	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", r.table, strings.Join(conds, " AND "))

	return r.scope.run(ctx, tid, func(q Querier) error {
		_, err := q.Exec(ctx, stmt, args...)
		return err
	})
}
