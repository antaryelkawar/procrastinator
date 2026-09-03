package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guard: the household repository satisfies the generic
// repository interface for its entity.
var _ repo.Repository[entity.Household] = (*HouseholdRepository)(nil)

// HouseholdRepository is the generic repository engine for entity.Household,
// extended with membership methods for the household_members join table
// (which has no owner_id column and is therefore not a target of the
// generic engine).
type HouseholdRepository struct {
	*pgRepository[entity.Household]
}

// NewHouseholdRepository returns a repository for entity.Household.
func NewHouseholdRepository(pool *pgxpool.Pool) *HouseholdRepository {
	return &HouseholdRepository{
		pgRepository: &pgRepository[entity.Household]{
			scope:   &poolScope{pool: pool},
			table:   "households",
			scanRow: scanHousehold,
			toMap:   householdToMap,
			filters: householdFilters,
		},
	}
}

// newHouseholdRepoForTx returns a household repository bound to an ambient transaction.
func newHouseholdRepoForTx(tx pgx.Tx) *HouseholdRepository {
	return &HouseholdRepository{
		pgRepository: &pgRepository[entity.Household]{
			scope:   &txScopeImpl{tx: tx},
			table:   "households",
			scanRow: scanHousehold,
			toMap:   householdToMap,
			filters: householdFilters,
		},
	}
}

// AddMember adds a user to a household, idempotently: an existing
// membership is left untouched.
func (r *HouseholdRepository) AddMember(ctx context.Context, householdID, userID string, opts ...repo.Option) error {
	tid, err := resolveOwner(ctx, repo.ApplyOptions(opts...))
	if err != nil {
		return err
	}
	return r.scope.run(ctx, tid, func(q Querier) error {
		_, err := q.Exec(ctx,
			`INSERT INTO household_members (household_id, user_id)
			 VALUES ($1, $2)
			 ON CONFLICT (household_id, user_id) DO NOTHING`,
			householdID, userID,
		)
		return err
	})
}

// ListMembers returns the members of a household, ordered by user id.
// Returns a non-nil empty slice when the household has no members.
func (r *HouseholdRepository) ListMembers(ctx context.Context, householdID string, opts ...repo.Option) ([]entity.HouseholdMember, error) {
	tid, err := resolveOwner(ctx, repo.ApplyOptions(opts...))
	if err != nil {
		return nil, err
	}

	var result []entity.HouseholdMember
	err = r.scope.run(ctx, tid, func(q Querier) error {
		rows, err := q.Query(ctx,
			`SELECT household_id, user_id, created_at
			 FROM household_members
			 WHERE household_id = $1
			 ORDER BY user_id`,
			householdID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.HouseholdMember, 0)
		for rows.Next() {
			var m entity.HouseholdMember
			if err := rows.Scan(&m.HouseholdID, &m.UserID, &m.CreatedAt); err != nil {
				return err
			}
			result = append(result, m)
		}
		return rows.Err()
	})
	return result, err
}

// HouseholdsForUser returns the ids of every household the given user is a
// member of. Returns a non-nil empty slice when the user belongs to no
// household.
func (r *HouseholdRepository) HouseholdsForUser(ctx context.Context, userID string, opts ...repo.Option) ([]string, error) {
	tid, err := resolveOwner(ctx, repo.ApplyOptions(opts...))
	if err != nil {
		return nil, err
	}

	var result []string
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var listErr error
		result, listErr = householdsForUser(ctx, q, userID)
		return listErr
	})
	return result, err
}

// householdVisibilityCond builds the member-aware visibility predicate for the
// households table:
//
//	visible(row, me) := owner_id = me OR id IN households(me)
//
// where households(me) is the set of household ids the user is a member of.
// Households have no owner_household_id column, so the generic shareable
// disjunct does not apply here; membership is expressed on the id column
// instead.
//
// The predicate is composable: it appends its bind arguments to *args (the
// user id first, then each household id) and returns the SQL fragment with
// $-placeholders that continue the caller's existing arg numbering. No id is
// ever interpolated literally. It never emits an empty IN (): when the user
// has no households the fragment is just "owner_id = $N".
func householdVisibilityCond(ctx context.Context, q Querier, userID string, args *[]any) (string, error) {
	// User id is always the first bind arg of the fragment.
	*args = append(*args, userID)
	ownerPos := len(*args)

	households, err := householdsForUser(ctx, q, userID)
	if err != nil {
		return "", err
	}
	if len(households) == 0 {
		// No memberships: only the user's own households are visible. No empty
		// IN () is emitted.
		return fmt.Sprintf("owner_id = $%d", ownerPos), nil
	}

	placeholders := make([]string, len(households))
	for i, id := range households {
		*args = append(*args, id)
		placeholders[i] = fmt.Sprintf("$%d", len(*args))
	}
	return fmt.Sprintf("(owner_id = $%d OR id IN (%s))", ownerPos, strings.Join(placeholders, ", ")), nil
}

// GetHouseholdVisible returns the household with the given id if it is visible
// to the requester (the requester is the owner or a member), otherwise
// repo.ErrNotFound. Unlike the generic Get, this applies the member-aware
// household visibility rule rather than owner-only scoping.
func (r *HouseholdRepository) GetHouseholdVisible(ctx context.Context, id string, opts ...repo.Option) (entity.Household, error) {
	var zero entity.Household
	o := repo.ApplyOptions(opts...)
	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return zero, err
	}

	var result entity.Household
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		args = append(args, id) // id = $1
		vis, err := householdVisibilityCond(ctx, q, tid, &args)
		if err != nil {
			return err
		}
		stmt := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 AND %s", r.table, vis)
		var scanErr error
		result, scanErr = r.scanRow(q.QueryRow(ctx, stmt, args...))
		return scanErr
	})
	return result, err
}

// ListHouseholdsVisible returns every household the requester owns or is a
// member of, in no particular order. The result is never nil. No arbitrary
// filters are supported: the visibility predicate is the only scoping, and the
// caller applies any higher-level ordering.
func (r *HouseholdRepository) ListHouseholdsVisible(ctx context.Context, opts ...repo.Option) ([]entity.Household, error) {
	o := repo.ApplyOptions(opts...)
	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.Household
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := householdVisibilityCond(ctx, q, tid, &args)
		if err != nil {
			return err
		}
		stmt := fmt.Sprintf("SELECT * FROM %s WHERE %s", r.table, vis)
		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.Household, 0)
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

// Exists reports whether a household with the given id exists, independent of
// the caller's membership. The households RLS policy hides rows the caller is
// not a member of, so a visibility-aware Get cannot distinguish "unknown
// household" (404) from "not a member" (403); this probe observes the row
// globally.
//
// It runs the EXISTS probe as the NOLOGIN BYPASSRLS role rls_bypass (a member
// of the app role, see initdb/01-app-role.sql), which bypasses RLS. rls_bypass
// holds no table privileges of its own, so the probe first grants it SELECT on
// households — idempotent, issued while still the app role (the table owner in
// the dev/test schemas; a no-op on every subsequent call). SET LOCAL ROLE is
// transaction-scoped, so the effective role reverts automatically when
// scope.run's transaction commits or rolls back.
func (r *HouseholdRepository) Exists(ctx context.Context, id string, opts ...repo.Option) (bool, error) {
	tid, err := resolveOwner(ctx, repo.ApplyOptions(opts...))
	if err != nil {
		return false, err
	}
	var exists bool
	err = r.scope.run(ctx, tid, func(q Querier) error {
		if _, err := q.Exec(ctx, `GRANT SELECT ON households TO rls_bypass`); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `SET LOCAL ROLE rls_bypass`); err != nil {
			return err
		}
		return q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM households WHERE id = $1)`, id).Scan(&exists)
	})
	return exists, err
}

var householdFieldCols = map[string]string{
	"display_name": "display_name",
	"owner_id":    "owner_id",
	"created_at":   "created_at",
}

var householdOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
}

var householdFilters = filterConfig{fieldCols: householdFieldCols, orderCols: householdOrderCols}

// scanHousehold scans a row into an entity.Household,
// mapping pgx.ErrNoRows to repo.ErrNotFound. Column order matches the
// households table (migration 00005_household_scope).
func scanHousehold(row rowScanner) (entity.Household, error) {
	var h entity.Household
	err := row.Scan(&h.ID, &h.OwnerID, &h.DisplayName, &h.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Household{}, repo.ErrNotFound
		}
		return entity.Household{}, err
	}
	return h, nil
}

func householdToMap(h entity.Household) map[string]any {
	m := make(map[string]any)
	if h.ID != "" {
		m["id"] = h.ID
	}
	if h.OwnerID != "" {
		m["owner_id"] = h.OwnerID
	}
	if h.DisplayName != "" {
		m["display_name"] = h.DisplayName
	}
	// created_at is intentionally omitted: the DB default (now()) applies.
	return m
}
