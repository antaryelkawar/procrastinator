package postgres

import (
	"context"
	"fmt"

	"procrastinator-backend/commons/repo"
)

// userRegistry implements repo.UserRegistry backed by the users table.
type userRegistry struct {
	q Querier
}

var _ repo.UserRegistry = (*userRegistry)(nil)

// NewUserRegistry returns a repo.UserRegistry that checks the users
// table for registration. The users table has no RLS, so no user binding
// is needed.
func NewUserRegistry(q Querier) repo.UserRegistry {
	return &userRegistry{q: q}
}

// Has reports whether the given user ID is registered in the users table.
func (r *userRegistry) Has(ctx context.Context, id string) (bool, error) {
	row := r.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: user registry: %w", err)
	}
	return exists, nil
}
