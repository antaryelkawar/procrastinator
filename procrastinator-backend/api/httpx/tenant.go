package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
)

// TenantMiddleware returns middleware that resolves the active user (tenant)
// for a request. The user ID is read from the {userId} URL path parameter.
//
// Transitional fallback: when the {userId} path parameter is empty, the
// middleware falls back to the X-Tenant-ID header. This exists because the
// /api/finance/... endpoints are a separate feature not yet migrated to
// URL-path tenancy and identify the user via the header; they must keep
// working until that migration lands.
//
// Decision table (semantics unchanged): empty/missing → 400, malformed → 400,
// registry error → 500, unregistered → 404. Registered tenants are stored in
// ctx via tenant.WithTenant.
func TenantMiddleware(reg repo.TenantRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "userId")
			if id == "" {
				// Transitional: the finance endpoints are not yet on
				// URL-path tenancy and still pass the user via the header.
				id = r.Header.Get("X-Tenant-ID")
			}
			if id == "" {
				WriteError(w, http.StatusBadRequest, "missing userId")
				return
			}
			if !tenant.Valid(id) {
				WriteError(w, http.StatusBadRequest, "malformed userId")
				return
			}
			registered, err := reg.Has(r.Context(), id)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "tenant lookup failed")
				return
			}
			if !registered {
				WriteError(w, http.StatusNotFound, "unknown user")
				return
			}
			ctx := tenant.WithTenant(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
