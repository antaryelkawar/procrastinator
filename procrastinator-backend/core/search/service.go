// Package search implements the tenant-scoped quick and paged search
// core service. It composes results from five owned entity types via
// repo.SearchBackend and applies the D-8 search contract: type priority
// ordering, ILIKE metacharacter escaping, and the Quick/Paged invariants.
package search

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Sentinel errors.
var (
	ErrQueryTooLong          = errors.New("query exceeds 200 characters")
	ErrInvalidLimit          = errors.New("limit must be between 1 and 50")
	ErrInvalidPage           = errors.New("page must be at least 1")
	ErrInvalidPageSize       = errors.New("page_size must be between 1 and 100")
	ErrInvalidWarrantyStatus = errors.New("invalid warranty_status value")
)

// Hit is one search result row, type-tagged.
type Hit struct {
	Type       string
	ID         string
	Title      string
	Subtitle   *string
	Confidence *float64
}

// Page is a paged search response.
type Page struct {
	Results  []Hit
	Page     int
	PageSize int
	Total    int
}

// Service is the search core service.
type Service struct {
	backend repo.SearchBackend
}

// New constructs a Service over the given search backend.
func New(backend repo.SearchBackend) *Service {
	return &Service{backend: backend}
}

// Quick returns the top-limit combined hits for q. Validation of limit and
// warranty_status happens before any backend call. Blank q returns an empty
// (non-nil) slice without touching the backend.
func (s *Service) Quick(ctx context.Context, q string, limit int, filters repo.Filters) ([]Hit, error) {
	if limit < 1 || limit > 50 {
		return nil, ErrInvalidLimit
	}
	if err := validateWarrantyStatus(filters.WarrantyStatus); err != nil {
		return nil, err
	}
	hits, err := s.allHits(ctx, q, filters)
	if err != nil {
		return nil, err
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// Paged returns one page of combined hits for q. Validation of page, pageSize,
// and warranty_status happens before any backend call. Blank q returns an empty
// page without touching the backend.
func (s *Service) Paged(ctx context.Context, q string, page, pageSize int, filters repo.Filters) (Page, error) {
	if page < 1 {
		return Page{}, ErrInvalidPage
	}
	if pageSize < 1 || pageSize > 100 {
		return Page{}, ErrInvalidPageSize
	}
	if err := validateWarrantyStatus(filters.WarrantyStatus); err != nil {
		return Page{}, err
	}
	hits, err := s.allHits(ctx, q, filters)
	if err != nil {
		return Page{}, err
	}
	total := len(hits)
	start := (page - 1) * pageSize
	if start >= total {
		return Page{Results: []Hit{}, Page: page, PageSize: pageSize, Total: total}, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return Page{Results: hits[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

// allHits performs the combined search: it trims q, validates length,
// builds the ILIKE pattern, invokes all five backend methods, and returns
// hits concatenated in type-priority order: assets, accounts, movements,
// documents, import_batches.
func (s *Service) allHits(ctx context.Context, q string, filters repo.Filters) ([]Hit, error) {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return []Hit{}, nil
	}
	if len(trimmed) > 200 {
		return nil, ErrQueryTooLong
	}
	pattern := buildPattern(trimmed)

	assets, err := s.backend.SearchAssets(ctx, pattern, filters)
	if err != nil {
		return nil, err
	}
	accounts, err := s.backend.SearchAccounts(ctx, pattern)
	if err != nil {
		return nil, err
	}
	movements, err := s.backend.SearchMovements(ctx, pattern)
	if err != nil {
		return nil, err
	}
	documents, err := s.backend.SearchDocuments(ctx, pattern, filters)
	if err != nil {
		return nil, err
	}
	batches, err := s.backend.SearchImportBatches(ctx, pattern)
	if err != nil {
		return nil, err
	}

	hits := make([]Hit, 0, len(assets)+len(accounts)+len(movements)+len(documents)+len(batches))
	for _, a := range assets {
		hits = append(hits, assetHit(a))
	}
	for _, a := range accounts {
		hits = append(hits, accountHit(a))
	}
	for _, m := range movements {
		hits = append(hits, movementHit(m))
	}
	for _, d := range documents {
		hits = append(hits, documentHit(d))
	}
	for _, b := range batches {
		hits = append(hits, importBatchHit(b))
	}
	return hits, nil
}

// buildPattern escapes the ILIKE metacharacters (\, %, _) in q so they match
// literally, and wraps the result in %...% for substring matching.
// Backslash is escaped first so the escapes themselves are not re-escaped.
func buildPattern(q string) string {
	var b strings.Builder
	for _, r := range q {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '%':
			b.WriteString(`\%`)
		case '_':
			b.WriteString(`\_`)
		default:
			b.WriteRune(r)
		}
	}
	return "%" + b.String() + "%"
}

// assetHit maps an Asset entity to a Hit.
func assetHit(a entity.Asset) Hit {
	var title string
	switch {
	case a.Brand != nil && a.Model != nil:
		title = *a.Brand + " " + *a.Model
	case a.Model != nil:
		title = *a.Model
	case a.SerialNumber != nil:
		title = *a.SerialNumber
	default:
		title = "asset"
	}
	var subtitle *string
	if a.SerialNumber != nil {
		s := *a.SerialNumber
		subtitle = &s
	}
	return Hit{
		Type:       "asset",
		ID:         a.ID,
		Title:      title,
		Subtitle:   subtitle,
		Confidence: a.Confidence,
	}
}

// accountHit maps a FinancialAccount entity to a Hit.
func accountHit(a entity.FinancialAccount) Hit {
	title := "account"
	if a.Name != "" {
		title = a.Name
	}
	var subtitle *string
	switch {
	case a.Institution != nil:
		s := a.Type + " · " + *a.Institution
		subtitle = &s
	case a.Type != "":
		s := a.Type
		subtitle = &s
	}
	return Hit{
		Type:     "account",
		ID:       a.ID,
		Title:    title,
		Subtitle: subtitle,
	}
}

// movementHit maps a MoneyMovement entity to a Hit.
func movementHit(m entity.MoneyMovement) Hit {
	title := "movement"
	if m.Description != "" {
		title = m.Description
	}
	var subtitle *string
	if m.ExternalReference != nil {
		s := *m.ExternalReference
		subtitle = &s
	}
	return Hit{
		Type:     "movement",
		ID:       m.ID,
		Title:    title,
		Subtitle: subtitle,
	}
}

// documentHit maps a Document entity to a Hit.
func documentHit(d entity.Document) Hit {
	title := "document"
	if d.DocType != "" {
		title = d.DocType
	}
	return Hit{
		Type:       "document",
		ID:         d.ID,
		Title:      title,
		Subtitle:   nil,
		Confidence: d.Confidence,
	}
}

// importBatchHit maps an ImportBatch entity to a Hit.
func importBatchHit(b entity.ImportBatch) Hit {
	title := "import_batch"
	if b.Filename != "" {
		title = b.Filename
	}
	return Hit{
		Type:   "import_batch",
		ID:     b.ID,
		Title:  title,
	}
}

// validateWarrantyStatus checks that a non-nil warranty_status filter value is
// syntactically valid. Accepted forms: "active", "expired", "expiring_within:N"
// where N is a positive integer. Returns ErrInvalidWarrantyStatus otherwise.
func validateWarrantyStatus(ws *string) error {
	if ws == nil {
		return nil
	}
	switch {
	case *ws == "active" || *ws == "expired":
		return nil
	case strings.HasPrefix(*ws, "expiring_within:"):
		days, err := strconv.Atoi(strings.TrimPrefix(*ws, "expiring_within:"))
		if err != nil || days < 1 {
			return ErrInvalidWarrantyStatus
		}
		return nil
	default:
		return ErrInvalidWarrantyStatus
	}
}
