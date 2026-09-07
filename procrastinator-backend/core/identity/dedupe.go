package identity

import (
	"context"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// Deduper resolves identity-level content-hash duplicates against the existing
// source and document repositories.
type Deduper struct {
	Sources   repo.Repository[entity.Source]
	Documents repo.Repository[entity.Document]
}

// NewDeduper constructs a Deduper over the given repositories.
func NewDeduper(sources repo.Repository[entity.Source], documents repo.Repository[entity.Document]) *Deduper {
	return &Deduper{Sources: sources, Documents: documents}
}

// DedupeHash performs an identity-level content-hash dedupe check. It returns
// the existing document already linked to assetID by a source whose content
// hash equals sourceHash, or (nil, false) when no such document exists.
//
// DedupeHash is strictly READ-ONLY (List calls only): by construction it
// cannot modify any asset field or create a second document. Both lookups are
// owner-scoped to the caller (from ctx) so a document from another tenant can
// never be returned; if no user is bound to ctx it fails closed with
// (nil, false). It returns no error.
func (d *Deduper) DedupeHash(ctx context.Context, sourceHash, assetID string) (*entity.Document, bool) {
	if sourceHash == "" || assetID == "" {
		return nil, false
	}
	// Owner scope: without a bound user we cannot resolve ownership safely.
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, false
	}

	srcs, err := d.Sources.List(ctx, repo.Where("sha256", "=", sourceHash), repo.Limit(1), repo.OrderBy("id"), repo.Owner(tid))
	if err != nil || len(srcs) == 0 {
		return nil, false
	}
	src := srcs[0]

	docs, err := d.Documents.List(ctx, repo.Where("asset_id", "=", assetID), repo.Where("source_id", "=", src.ID), repo.Limit(1), repo.OrderBy("id"), repo.Owner(tid))
	if err != nil || len(docs) == 0 {
		return nil, false
	}
	doc := docs[0]
	return &doc, true
}
