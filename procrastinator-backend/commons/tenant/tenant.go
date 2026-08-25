package tenant

import (
	"context"
	"errors"
	"regexp"
)

// ErrNoTenant is returned when a context carries no tenant ID.
var ErrNoTenant = errors.New("no tenant in context")

// tenantKey is the unexported key type for storing tenant ID in context.
type tenantKey struct{}

// tenantPattern validates tenant IDs: 1-64 characters, alphanumeric plus underscore and hyphen.
var tenantPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// WithTenant returns a child context carrying the given tenant ID.
// If the ID is invalid (does not match the pattern), the original context is returned unchanged.
func WithTenant(ctx context.Context, id string) context.Context {
	if !tenantPattern.MatchString(id) {
		return ctx
	}
	return context.WithValue(ctx, tenantKey{}, id)
}

// TenantFrom extracts the tenant ID from the context.
// Returns ErrNoTenant if no tenant is present or if the value is not a string.
func TenantFrom(ctx context.Context) (string, error) {
	val := ctx.Value(tenantKey{})
	if val == nil {
		return "", ErrNoTenant
	}
	id, ok := val.(string)
	if !ok {
		return "", ErrNoTenant
	}
	return id, nil
}
