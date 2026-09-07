package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// DetachDocumentsByAsset clears the asset link (asset_id) on every document
// currently linked to assetID, retaining the document rows and their source
// rows. It is the documents-link-null helper used by the asset lifecycle
// service (purge). Owner scoping is taken from opts (pass Owner(id) so only
// the caller's own linked documents are touched). It returns the number of
// documents whose link was cleared.
func DetachDocumentsByAsset(ctx context.Context, docs Repository[entity.Document], assetID string, opts ...Option) (int, error) {
	listOpts := append([]Option{Where("asset_id", "=", assetID)}, opts...)
	linked, err := docs.List(ctx, listOpts...)
	if err != nil {
		return 0, err
	}
	for _, d := range linked {
		d.AssetID = ""
		updOpts := append(append([]Option{}, opts...), Set("asset_id", nil))
		if _, err := docs.Update(ctx, d, updOpts...); err != nil {
			return 0, err
		}
	}
	return len(linked), nil
}
