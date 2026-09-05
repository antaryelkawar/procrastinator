package api

import (
	"context"
	"errors"
	"net/http"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/core/search"
)

// QuickSearch returns the top-N combined search hits for q (typeahead).
func (s *Server) QuickSearch(ctx context.Context, request gen.QuickSearchRequestObject) (gen.QuickSearchResponseObject, error) {
	q := ""
	if request.Params.Q != nil {
		q = *request.Params.Q
	}
	limit := 10
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	hits, err := s.search.Quick(ctx, q, limit)
	if err != nil {
		return nil, mapSearchError(err)
	}
	return gen.QuickSearch200JSONResponse(toSearchQuickResponse(hits)), nil
}

// Search returns one page of combined search hits for q.
func (s *Server) Search(ctx context.Context, request gen.SearchRequestObject) (gen.SearchResponseObject, error) {
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
	result, err := s.search.Paged(ctx, q, page, pageSize)
	if err != nil {
		return nil, mapSearchError(err)
	}
	return gen.Search200JSONResponse(toSearchResultsPage(result)), nil
}

// mapSearchError maps core/search sentinels to API errors.
func mapSearchError(err error) error {
	switch {
	case errors.Is(err, search.ErrQueryTooLong):
		return newAPIError(http.StatusBadRequest, "query exceeds 200 characters")
	case errors.Is(err, search.ErrInvalidLimit):
		return newAPIError(http.StatusBadRequest, "limit must be between 1 and 50")
	case errors.Is(err, search.ErrInvalidPage):
		return newAPIError(http.StatusBadRequest, "page must be at least 1")
	case errors.Is(err, search.ErrInvalidPageSize):
		return newAPIError(http.StatusBadRequest, "page_size must be between 1 and 100")
	default:
		return newAPIError(http.StatusInternalServerError, "internal error")
	}
}
