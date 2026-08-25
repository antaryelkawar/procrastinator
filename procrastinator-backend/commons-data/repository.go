package data

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("entity not found")

// AssetRepository provides tenant-scoped asset persistence operations.
// Tenant is extracted from ctx by the implementation.
type AssetRepository interface {
	Create(ctx context.Context, a Asset) (Asset, error)
	CreateOnConflictSerial(ctx context.Context, a Asset) (Asset, bool, error)
	GetByID(ctx context.Context, id string) (Asset, error)
	List(ctx context.Context) ([]Asset, error)
	FindBySerial(ctx context.Context, normSerial string) (Asset, error)
	FindByBrandModel(ctx context.Context, normBrand, normModel string) (Asset, error)
	UpdateFields(ctx context.Context, id string, f UpdateFields) (Asset, error)
}

// SourceRepository provides tenant-scoped source (uploaded file) persistence.
type SourceRepository interface {
	Create(ctx context.Context, s Source) (Source, error)
	GetByID(ctx context.Context, id string) (Source, error)
}

// DocumentRepository provides tenant-scoped document persistence.
type DocumentRepository interface {
	Create(ctx context.Context, d Document) (Document, error)
	GetByID(ctx context.Context, id string) (Document, error)
	ListByAsset(ctx context.Context, assetID string) ([]DocumentWithSource, error)
}
