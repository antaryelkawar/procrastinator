package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// Extractor extracts structured data from an uploaded document via LLM.
// Returns the raw LLM response text (not yet parsed).
type Extractor interface {
	Extract(ctx context.Context, contentType string, data []byte) (string, error)
}

// FileStorage stores uploaded file bytes and returns the Source record.
type FileStorage interface {
	Put(ctx context.Context, data []byte) (entity.Source, error)
}
