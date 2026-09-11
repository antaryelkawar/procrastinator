package search

import (
	"context"
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	coresearch "procrastinator-backend/core/search"
)

// Service serves the search endpoints (quick + paged).
type Service struct {
	search *coresearch.Service
}

// New constructs a Service.
func New(search *coresearch.Service) *Service {
	return &Service{search: search}
}

// QuickSearch returns the top-N combined search hits for q (typeahead).
func (s *Service) QuickSearch(ctx context.Context, request gen.QuickSearchRequestObject) (gen.QuickSearchResponseObject, error) {
	q := ""
	if request.Params.Q != nil {
		q = *request.Params.Q
	}
	limit := 10
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	filters := bindQuickSearchFilters(&request.Params)
	hits, err := s.search.Quick(ctx, q, limit, filters)
	if err != nil {
		return nil, mapSearchError(err)
	}
	return gen.QuickSearch200JSONResponse(dto.ToSearchQuickResponse(hits)), nil
}

// Search returns one page of combined search hits for q.
func (s *Service) Search(ctx context.Context, request gen.SearchRequestObject) (gen.SearchResponseObject, error) {
	q := ""
	if request.Params.Q != nil {
		q = *request.Params.Q
	}
	page := 1
	if request.Params.Page != nil {
		page = *request.Params.Page
	}
	pageSize := 20
	if request.Params.PageSize != nil {
		pageSize = *request.Params.PageSize
	}
	filters := bindSearchFilters(&request.Params)
	result, err := s.search.Paged(ctx, q, page, pageSize, filters)
	if err != nil {
		return nil, mapSearchError(err)
	}
	return gen.Search200JSONResponse(dto.ToSearchResultsPage(result)), nil
}

// mapSearchError maps core/search sentinels to API errors.
func mapSearchError(err error) error {
	switch {
	case errors.Is(err, coresearch.ErrQueryTooLong):
		return httpx.NewAPIError(http.StatusBadRequest, "query exceeds 200 characters")
	case errors.Is(err, coresearch.ErrInvalidLimit):
		return httpx.NewAPIError(http.StatusBadRequest, "limit must be between 1 and 50")
	case errors.Is(err, coresearch.ErrInvalidPage):
		return httpx.NewAPIError(http.StatusBadRequest, "page must be at least 1")
	case errors.Is(err, coresearch.ErrInvalidPageSize):
		return httpx.NewAPIError(http.StatusBadRequest, "page_size must be between 1 and 100")
	case errors.Is(err, coresearch.ErrInvalidWarrantyStatus):
		return httpx.NewAPIError(http.StatusBadRequest, "warranty_status must be 'active', 'expired', or 'expiring_within:N'")
	default:
		return httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
}

// bindSearchFilters extracts the structured search filters from the paged
// search params.
func bindSearchFilters(p *gen.SearchParams) repo.Filters {
	return buildFilters(p.Category, p.Brand, p.PurchaseFrom, p.PurchaseTo, p.WarrantyStatus, p.HasDocuments, docClassPtr(p.DocClassification))
}

// bindQuickSearchFilters extracts the structured search filters from the
// quick-search params.
func bindQuickSearchFilters(p *gen.QuickSearchParams) repo.Filters {
	return buildFilters(p.Category, p.Brand, p.PurchaseFrom, p.PurchaseTo, p.WarrantyStatus, p.HasDocuments, docClassPtr(p.DocClassification))
}

// buildFilters assembles a repo.Filters from the individual param fields.
func buildFilters(category, brand *string, purchaseFrom, purchaseTo *openapi_types.Date, warrantyStatus *string, hasDocuments *bool, docClass *string) repo.Filters {
	var f repo.Filters
	f.Category = category
	f.Brand = brand
	if purchaseFrom != nil {
		t := purchaseFrom.Time
		f.PurchaseFrom = &t
	}
	if purchaseTo != nil {
		t := purchaseTo.Time
		f.PurchaseTo = &t
	}
	f.WarrantyStatus = warrantyStatus
	f.HasDocuments = hasDocuments
	f.DocClassification = docClass
	return f
}

// docClassPtr converts a *T (where T is a string-based type) to *string,
// returning nil for nil input.
func docClassPtr[T ~string](p *T) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}
