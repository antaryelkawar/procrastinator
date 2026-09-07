package api

import (
	"context"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/search"
)

// toAsset converts a entity.Asset into the generated gen.Asset DTO. Metadata is
// initialized to an empty map so it marshals as {} rather than null.
func toAsset(a entity.Asset) gen.Asset {
	metadata := a.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	var confidence *float32
	if a.Confidence != nil {
		c := float32(*a.Confidence)
		confidence = &c
	}
	return gen.Asset{
		Id:               a.ID,
		Brand:            a.Brand,
		Model:            a.Model,
		SerialNumber:     a.SerialNumber,
		PurchaseDate:     a.PurchaseDate,
		WarrantyEnd:      a.WarrantyEnd,
		Price:            a.Price,
		Currency:         a.Currency,
		Metadata:         metadata,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
		OwnerHouseholdId: a.OwnerHouseholdID,
		Confidence:       confidence,
		AssetCategory:      assetCategoryPtr(a.AssetCategory),
		CategoryConfidence: catConfPtr(a.CategoryConfidence),
		DeletedAt:          a.DeletedAt,
		MergedInto:         a.MergedInto,
		MergedAt:           a.MergedAt,
		Name:               a.Name,
	}
}

// assetCategoryPtr converts *string to *gen.AssetAssetCategory (nil-safe).
func assetCategoryPtr(s *string) *gen.AssetAssetCategory {
	if s == nil {
		return nil
	}
	c := gen.AssetAssetCategory(*s)
	return &c
}

// catConfPtr converts *float64 to *float32 (nil-safe).
func catConfPtr(p *float64) *float32 {
	if p == nil {
		return nil
	}
	c := float32(*p)
	return &c
}

// toDocument converts a entity.DocumentWithSource into the generated gen.Document DTO.
func toDocument(d entity.DocumentWithSource) gen.Document {
	var confidence *float32
	if d.Confidence != nil {
		c := float32(*d.Confidence)
		confidence = &c
	}
	return gen.Document{
		Id:               d.ID,
		DocType:          gen.DocumentDocType(d.DocType),
		SourceFilename:   d.SourceFilename,
		SourceUploadedAt: d.SourceUploadedAt,
		CreatedAt:        d.CreatedAt,
		OwnerHouseholdId: d.OwnerHouseholdID,
		Confidence:       confidence,
	}
}

// toReview converts an entity.IngestReview into the generated gen.IngestReview
// DTO. It fetches the source (for filename/uploaded_at) and optionally the
// best-matched asset (for the title) under the user's scope.
func (s *Server) toReview(ctx context.Context, tid string, rev entity.IngestReview) (gen.IngestReview, error) {
	srcs, err := s.factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", rev.SourceID))
	if err != nil {
		return gen.IngestReview{}, err
	}
	if len(srcs) == 0 {
		return gen.IngestReview{}, repo.ErrNotFound
	}
	src := srcs[0]

	cf := rev.CandidateFields
	if cf == nil {
		cf = make(map[string]any)
	}

	dto := gen.IngestReview{
		Id:                 rev.ID,
		DocType:            rev.DocType,
		CandidateFields:    cf,
		State:              gen.IngestReviewState(rev.State),
		CreatedAt:          rev.CreatedAt,
		SourceFilename:     src.Filename,
		SourceUploadedAt:   src.UploadedAt,
		BestMatchedAssetId: rev.BestMatchedAssetID,
		DecidedAt:          rev.DecidedAt,
	}
	if rev.Confidence != nil {
		c := float32(*rev.Confidence)
		dto.Confidence = &c
	}
	if rev.BestMatchedAssetID != nil {
		assets, err := s.factory.Assets.List(ctx, repo.Owner(tid), repo.Where("id", "=", *rev.BestMatchedAssetID))
		if err != nil {
			return gen.IngestReview{}, err
		}
		if len(assets) > 0 {
			a := assets[0]
			title := reviewAssetTitle(&a)
			dto.BestMatchedAssetTitle = &title
		}
	}
	return dto, nil
}

// reviewAssetTitle derives a display title from an asset: brand+model if both
// present, model if only model, serial if only serial, else "asset".
func reviewAssetTitle(a *entity.Asset) string {
	switch {
	case a.Brand != nil && a.Model != nil:
		return *a.Brand + " " + *a.Model
	case a.Model != nil:
		return *a.Model
	case a.SerialNumber != nil:
		return *a.SerialNumber
	default:
		return "asset"
	}
}

// toSearchHit converts a search.Hit into the generated gen.SearchHit DTO.
func toSearchHit(h search.Hit) gen.SearchHit {
	dto := gen.SearchHit{
		Id:       h.ID,
		Title:    h.Title,
		Type:     gen.SearchHitType(h.Type),
		Subtitle: h.Subtitle,
	}
	if h.Confidence != nil {
		c := float32(*h.Confidence)
		dto.Confidence = &c
	}
	return dto
}

// toSearchQuickResponse converts a slice of search.Hit into the generated
// gen.SearchQuickResponse DTO.
func toSearchQuickResponse(hits []search.Hit) gen.SearchQuickResponse {
	results := make([]gen.SearchHit, 0, len(hits))
	for _, h := range hits {
		results = append(results, toSearchHit(h))
	}
	return gen.SearchQuickResponse{Results: results}
}

// toSearchResultsPage converts a search.Page into the generated
// gen.SearchResultsPage DTO.
func toSearchResultsPage(p search.Page) gen.SearchResultsPage {
	results := make([]gen.SearchHit, 0, len(p.Results))
	for _, h := range p.Results {
		results = append(results, toSearchHit(h))
	}
	return gen.SearchResultsPage{
		Results:  results,
		Page:     p.Page,
		PageSize: p.PageSize,
		Total:    p.Total,
	}
}
