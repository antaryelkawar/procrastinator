package processing

import (
	"context"
	"errors"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// DedupeSource performs a source-level content-hash dedupe lookup. Before any
// extraction, storage, or LLM work, it checks whether the owner already has a
// source with the given SHA256 hash. If so, it returns an OutcomeDuplicate
// referencing the existing source, document, and asset (flagging soft-deleted
// assets so the UI can offer restore). This function stores no new source and
// makes no LLM call; the caller skips extraction entirely when ok is true.
func DedupeSource(ctx context.Context,
	sources repo.Repository[entity.Source],
	documents repo.Repository[entity.Document],
	assets repo.Repository[entity.Asset],
	hash string,
) (out Outcome, ok bool, err error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return Outcome{}, false, err
	}

	if hash == "" {
		return Outcome{}, false, nil
	}

	srcs, err := sources.List(ctx, repo.Where("sha256", "=", hash), repo.Limit(1), repo.OrderBy("id"), repo.Owner(tid))
	if err != nil {
		return Outcome{}, false, err
	}
	if len(srcs) == 0 {
		return Outcome{}, false, nil
	}

	src := srcs[0]

	docs, err := documents.List(ctx, repo.Where("source_id", "=", src.ID), repo.Limit(1), repo.OrderBy("id"), repo.Owner(tid))
	if err != nil {
		return Outcome{}, false, err
	}

	docID, assetID := "", ""
	if len(docs) > 0 {
		docID = docs[0].ID
		assetID = docs[0].AssetID
	}

	assetDeleted := false
	if assetID != "" {
		a, aerr := assets.Get(ctx, assetID, repo.Owner(tid))
		if aerr == nil {
			assetDeleted = a.DeletedAt != nil
		} else if !errors.Is(aerr, repo.ErrNotFound) {
			return Outcome{}, false, aerr
		}
	}

	dup := &Duplicate{
		SourceID:         src.ID,
		DocumentID:       docID,
		AssetID:          assetID,
		AssetDeleted:     assetDeleted,
		SourceFilename:   src.Filename,
		SourceUploadedAt: src.UploadedAt,
	}
	return Outcome{Kind: OutcomeDuplicate, Duplicate: dup}, true, nil
}
