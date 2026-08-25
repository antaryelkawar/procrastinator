package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/commons-server/httpx"
	"procrastinator-backend/procrastinator-core/ingest"
)

// Server serves the HTTP API surface over the ingest service and the
// tenant-scoped data repositories.
type Server struct {
	svc      *ingest.Service
	assets   data.AssetRepository
	docs     data.DocumentRepository
	maxBytes int64
}

// New constructs a Server. maxBytes is the maximum accepted upload size in
// bytes; it is enforced both at the HTTP layer (MaxBytesReader) and inside
// the ingest service.
func New(svc *ingest.Service, assets data.AssetRepository, docs data.DocumentRepository, maxBytes int64) *Server {
	return &Server{
		svc:      svc,
		assets:   assets,
		docs:     docs,
		maxBytes: maxBytes,
	}
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
// the context key used by the data repositories. httpx and commons-data each
// use their own unexported context key, and data.WithTenant silently drops
// IDs that do not match its validation pattern; without this bridge every
// repository call would fail with data.ErrNoTenant. The middleware runs
// after httpx.TenantMiddleware and fails closed for both missing and
// invalid tenant IDs.
func (s *Server) tenantContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := httpx.TenantFrom(r.Context())
		if !ok {
			// Defensive: httpx.TenantMiddleware already rejects missing
			// headers, so this is unreachable in the normal chain.
			httpx.WriteError(w, http.StatusUnauthorized, "missing or empty X-Tenant-ID header")
			return
		}

		ctx := data.WithTenant(r.Context(), tenant)
		if _, err := data.TenantFrom(ctx); err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid X-Tenant-ID header")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
