package postgres

import (
	"context"
	"errors"

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
// (which has no tenant_id column and is therefore not a target of the
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
	tid, err := resolveTenant(ctx, repo.ApplyOptions(opts...))
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
	tid, err := resolveTenant(ctx, repo.ApplyOptions(opts...))
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
	tid, err := resolveTenant(ctx, repo.ApplyOptions(opts...))
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

var householdFieldCols = map[string]string{
	"display_name": "display_name",
	"tenant_id":    "tenant_id",
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
	err := row.Scan(&h.ID, &h.TenantID, &h.DisplayName, &h.CreatedAt)
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
	if h.TenantID != "" {
		m["tenant_id"] = h.TenantID
	}
	if h.DisplayName != "" {
		m["display_name"] = h.DisplayName
	}
	// created_at is intentionally omitted: the DB default (now()) applies.
	return m
}
