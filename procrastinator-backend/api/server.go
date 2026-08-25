package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
	"procrastinator-backend/core/ingest"
)

// Server serves the HTTP API surface over the ingest service and the
// tenant-scoped data repositories.
type Server struct {
	svc      *ingest.Service
	factory  *repo.Factory
	maxBytes int64
}

// New constructs a Server. maxBytes is the maximum accepted upload size in
// bytes; it is enforced both at the HTTP layer (MaxBytesReader) and inside
// the ingest service.
func New(svc *ingest.Service, factory *repo.Factory, maxBytes int64) *Server {
	return &Server{
		svc:      svc,
		factory:  factory,
		maxBytes: maxBytes,
	}
}

// tenantFromCtx extracts the tenant ID from ctx, failing the request with a
// 401 when the tenant is missing or invalid. The tenantContext middleware
// normally guarantees a valid tenant, so a failure here is defensive.
func tenantFromCtx(w http.ResponseWriter, ctx context.Context) (string, bool) {
	tid, err := tenant.TenantFrom(ctx)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid X-Tenant-ID header")
		return "", false
	}
	return tid, true
}

// listDocumentsWithSource returns the documents attached to assetID joined
// with their source metadata, ordered by created_at then id.
func (s *Server) listDocumentsWithSource(ctx context.Context, tid, assetID string) ([]entity.DocumentWithSource, error) {
	list, err := s.factory.Documents.List(ctx, repo.Tenant(tid), repo.Where("asset_id", "=", assetID), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return []entity.DocumentWithSource{}, nil
	}
	out := make([]entity.DocumentWithSource, 0, len(list))
	for _, d := range list {
		src, err := s.factory.Sources.Get(ctx, d.SourceID, repo.Tenant(tid))
		if err != nil {
			return nil, err
		}
		dw := entity.DocumentWithSource{Document: d}
		dw.SourceFilename = src.Filename
		dw.SourceUploadedAt = src.UploadedAt
		out = append(out, dw)
	}
	return out, nil
}

// Routes returns the fully-wired router for the API.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.TenantMiddleware)
	r.Use(s.tenantContext)

	r.Post("/api/documents", s.handleUpload)
	r.Get("/api/assets", s.handleListAssets)
	r.Get("/api/assets/{id}", s.handleGetAsset)
	r.Get("/api/assets/{id}/documents", s.handleListDocuments)

	return r
}

// tenantContext bridges the tenant ID stored by httpx.TenantMiddleware into
// the context key used by the repositories. httpx and tenant each
// use their own unexported context key, and tenant.WithTenant silently drops
// IDs that do not match its validation pattern; without this bridge every
// repository call would fail with tenant.ErrNoTenant. The middleware runs
// after httpx.TenantMiddleware and fails closed for both missing and
// invalid tenant IDs.
func (s *Server) tenantContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := httpx.TenantFrom(r.Context())
		if !ok {
			// Defensive: httpx.TenantMiddleware already rejects missing
			// headers, so this is unreachable in the normal chain.
			httpx.WriteError(w, http.StatusUnauthorized, "missing or empty X-Tenant-ID header")
			return
		}

		ctx := tenant.WithTenant(r.Context(), tenantID)
		if _, err := tenant.TenantFrom(ctx); err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid X-Tenant-ID header")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
