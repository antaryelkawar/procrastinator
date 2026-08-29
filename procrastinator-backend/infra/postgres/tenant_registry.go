package postgres

import (
	"context"
	"fmt"

	"procrastinator-backend/commons/repo"
)

// tenantRegistry implements repo.TenantRegistry backed by the tenants table.
type tenantRegistry struct {
	q Querier
}

var _ repo.TenantRegistry = (*tenantRegistry)(nil)

// NewTenantRegistry returns a repo.TenantRegistry that checks the tenants
// table for registration. The tenants table has no RLS, so no tenant binding
// is needed.
func NewTenantRegistry(q Querier) repo.TenantRegistry {
	return &tenantRegistry{q: q}
}

// Has reports whether the given tenant ID is registered in the tenants table.
func (r *tenantRegistry) Has(ctx context.Context, id string) (bool, error) {
	row := r.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)`, id)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: tenant registry: %w", err)
	}
	return exists, nil
}
