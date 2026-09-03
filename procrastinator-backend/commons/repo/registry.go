package repo

import "context"

// UserRegistry is the lookup boundary for the user registry (users table).
// Middleware uses it to reject unregistered users before any handler runs.
type UserRegistry interface {
	Has(ctx context.Context, id string) (bool, error)
}
