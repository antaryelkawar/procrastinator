package dto

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/search"
)

// ToAsset converts a entity.Asset into the generated gen.Asset DTO. The payload
// data fields live under the nested `data` object (entity-shaped wire, design D7).
// Metadata is initialized to an empty map so it marshals as {} rather than null.
func ToAsset(a entity.Asset) gen.Asset {
	metadata := a.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	var confidence *float32
	if a.Confidence != nil {
		c := float32(*a.Confidence)
		confidence = &c
	}
	categoryUserSet := a.CategoryUserSet
	return gen.Asset{
		Id:               a.ID,
		OwnerHouseholdId: a.OwnerHouseholdID,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
		Data: struct {
			AssetCategory      *gen.AssetDataAssetCategory `json:"asset_category,omitempty"`
			Brand              *string                     `json:"brand,omitempty"`
			CategoryConfidence *float32                    `json:"category_confidence,omitempty"`
			CategoryUserSet    *bool                       `json:"category_user_set,omitempty"`
			Confidence         *float32                    `json:"confidence,omitempty"`
			Currency           *string                     `json:"currency,omitempty"`
			DeletedAt          *time.Time                  `json:"deleted_at,omitempty"`
			MergedAssets       *[]struct {
				AssetId  string    `json:"asset_id"`
				MergedAt time.Time `json:"merged_at"`
			} `json:"merged_assets,omitempty"`
			MergedAt     *time.Time               `json:"merged_at,omitempty"`
			MergedInto   *string                  `json:"merged_into,omitempty"`
			Metadata     map[string]interface{}   `json:"metadata"`
			Model        *string                  `json:"model,omitempty"`
			Name         *string                  `json:"name,omitempty"`
			Price        *string                  `json:"price,omitempty"`
			PurchaseDate *openapi_types.Date      `json:"purchase_date,omitempty"`
			SerialNumber *string                  `json:"serial_number,omitempty"`
			WarrantyEnd  *openapi_types.Date      `json:"warranty_end,omitempty"`
		}{
			Name:               a.Name,
			Brand:              a.Brand,
			Model:              a.Model,
			SerialNumber:       a.SerialNumber,
			AssetCategory:      AssetDataCategoryPtr(a.AssetCategory),
			CategoryConfidence: CatConfPtr(a.CategoryConfidence),
			CategoryUserSet:    &categoryUserSet,
			PurchaseDate:       DatePtr(a.PurchaseDate),
			WarrantyEnd:        DatePtr(a.WarrantyEnd),
			Price:              a.Price,
			Currency:           a.Currency,
			Metadata:           metadata,
			Confidence:         confidence,
			DeletedAt:          a.DeletedAt,
			MergedInto:         a.MergedInto,
			MergedAt:           a.MergedAt,
		},
	}
}

// AssetDataCategoryPtr converts *string to *gen.AssetDataAssetCategory (nil-safe).
func AssetDataCategoryPtr(s *string) *gen.AssetDataAssetCategory {
	if s == nil {
		return nil
	}
	c := gen.AssetDataAssetCategory(*s)
	return &c
}

// CatConfPtr converts *float64 to *float32 (nil-safe).
func CatConfPtr(p *float64) *float32 {
	if p == nil {
		return nil
	}
	c := float32(*p)
	return &c
}

// DatePtr converts a *time.Time into a *openapi_types.Date (nil-safe). Dates are
// stored in the payload as "2006-01-02" and render via openapi_types.Date.
func DatePtr(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	d := openapi_types.Date{Time: *t}
	return &d
}

// ToDocument converts a entity.DocumentWithSource into the generated gen.Document
// DTO. The payload data fields live under the nested `data` object. status is
// the derived processing status (asset linked / in review / asset-less /
// failed); the caller derives it from the document and its pending review.
func ToDocument(d entity.DocumentWithSource, status gen.DocumentStatus) gen.Document {
	var confidence *float32
	if d.Confidence != nil {
		c := float32(*d.Confidence)
		confidence = &c
	}
	extractedFields := d.ExtractedFields
	rawExtraction := d.RawExtraction
	userDirective := d.UserDirective
	// asset_id is null when detached (no linked asset) rather than "".
	var assetID *string
	if d.AssetID != "" {
		assetID = &d.AssetID
	}
	return gen.Document{
		Id:               d.ID,
		AssetId:          assetID,
		SourceId:         d.SourceID,
		OwnerHouseholdId: d.OwnerHouseholdID,
		SourceFilename:   d.SourceFilename,
		SourceUploadedAt: d.SourceUploadedAt,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
		Status:           status,
		Data: struct {
			Confidence      *float32                `json:"confidence,omitempty"`
			DocType         gen.DocumentDataDocType `json:"doc_type"`
			ExtractedFields *map[string]interface{} `json:"extracted_fields,omitempty"`
			RawExtraction   *string                 `json:"raw_extraction,omitempty"`
			UserDirective   *string                 `json:"user_directive,omitempty"`
		}{
			DocType:         gen.DocumentDataDocType(d.DocType),
			Confidence:      confidence,
			UserDirective:   &userDirective,
			ExtractedFields: &extractedFields,
			RawExtraction:   &rawExtraction,
		},
	}
}

// FindAssetScoped returns the single asset with id visible to the user under
// the scope access rule, or nil when the user cannot see it (unknown or
// another user's assets both surface as not-found).
func FindAssetScoped(factory *repo.Factory, ctx context.Context, tid, id string) (*entity.Asset, error) {
	assets, err := factory.Assets.List(ctx, repo.Owner(tid), repo.Where("id", "=", id))
	if err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, nil
	}
	a := assets[0]
	return &a, nil
}

// ToReview converts an entity.IngestReview into the generated gen.IngestReview
// DTO. It fetches the source (for filename/uploaded_at) and optionally the
// best-matched asset (for the title) under the user's scope.
func ToReview(factory *repo.Factory, ctx context.Context, tid string, rev entity.IngestReview) (gen.IngestReview, error) {
	srcs, err := factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", rev.SourceID))
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

	var confidence *float32
	if rev.Confidence != nil {
		c := float32(*rev.Confidence)
		confidence = &c
	}
	rawExtraction := rev.RawExtraction
	provenance := rev.Provenance

	dto := gen.IngestReview{
		Id:               rev.ID,
		SourceId:         rev.SourceID,
		OwnerHouseholdId: rev.OwnerHouseholdID,
		SourceFilename:   src.Filename,
		SourceUploadedAt: src.UploadedAt,
		CreatedAt:        rev.CreatedAt,
		UpdatedAt:        rev.UpdatedAt,
		Data: struct {
			BestMatchedAssetId *string                  `json:"best_matched_asset_id,omitempty"`
			CandidateFields    map[string]interface{}   `json:"candidate_fields"`
			Confidence         *float32                 `json:"confidence,omitempty"`
			DecidedAt          *time.Time               `json:"decided_at,omitempty"`
			DecidedBy          *string                  `json:"decided_by,omitempty"`
			DocType            string                   `json:"doc_type"`
			Provenance         *map[string]interface{}  `json:"provenance,omitempty"`
			RawExtraction      *string                  `json:"raw_extraction,omitempty"`
			State              gen.IngestReviewDataState `json:"state"`
		}{
			DocType:            rev.DocType,
			Confidence:         confidence,
			CandidateFields:    cf,
			State:              gen.IngestReviewDataState(rev.State),
			BestMatchedAssetId: rev.BestMatchedAssetID,
			DecidedAt:          rev.DecidedAt,
			DecidedBy:          rev.DecidedBy,
			RawExtraction:      &rawExtraction,
			Provenance:         &provenance,
		},
	}
	if rev.BestMatchedAssetID != nil {
		assets, err := factory.Assets.List(ctx, repo.Owner(tid), repo.Where("id", "=", *rev.BestMatchedAssetID))
		if err != nil {
			return gen.IngestReview{}, err
		}
		if len(assets) > 0 {
			a := assets[0]
			title := ReviewAssetTitle(&a)
			dto.BestMatchedAssetTitle = &title
		}
	}
	return dto, nil
}

// ReviewAssetTitle derives a display title from an asset: brand+model if both
// present, model if only model, serial if only serial, else "asset".
func ReviewAssetTitle(a *entity.Asset) string {
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

// ToSearchHit converts a search.Hit into the generated gen.SearchHit DTO.
func ToSearchHit(h search.Hit) gen.SearchHit {
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

// ToSearchQuickResponse converts a slice of search.Hit into the generated
// gen.SearchQuickResponse DTO.
func ToSearchQuickResponse(hits []search.Hit) gen.SearchQuickResponse {
	results := make([]gen.SearchHit, 0, len(hits))
	for _, h := range hits {
		results = append(results, ToSearchHit(h))
	}
	return gen.SearchQuickResponse{Results: results}
}

// ToSearchResultsPage converts a search.Page into the generated
// gen.SearchResultsPage DTO.
func ToSearchResultsPage(p search.Page) gen.SearchResultsPage {
	results := make([]gen.SearchHit, 0, len(p.Results))
	for _, h := range p.Results {
		results = append(results, ToSearchHit(h))
	}
	return gen.SearchResultsPage{
		Results:  results,
		Page:     p.Page,
		PageSize: p.PageSize,
		Total:    p.Total,
	}
}
