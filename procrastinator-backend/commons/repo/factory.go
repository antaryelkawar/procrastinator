package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// Factory provides access to all repositories and transactional scoping.
// This is the v3 replacement for the legacy per-entity factory interface.
type Factory struct {
	Assets    Repository[entity.Asset]
	Sources   Repository[entity.Source]
	Documents Repository[entity.Document]
	InTx      func(ctx context.Context, fn func(ctx context.Context, repos *Repos) error) error
}

// Repos is a bundle of repository handles passed to transaction callbacks.
// It mirrors the concrete fields of Factory for use inside InTx.
type Repos struct {
	Assets    Repository[entity.Asset]
	Sources   Repository[entity.Source]
	Documents Repository[entity.Document]
}
