package repo

import "context"

// TenantRegistry is the lookup boundary for the tenant registry (tenants table).
// Middleware uses it to reject unregistered tenants before any handler runs.
type TenantRegistry interface {
	Has(ctx context.Context, id string) (bool, error)
}
