package httpx

import (
	"context"
	"net/http"
)

// ctxKey is an unexported type used for context value keys in this package,
// preventing collisions with keys defined in other packages.
type ctxKey int

const tenantKey ctxKey = 0

// TenantMiddleware returns an http.Handler that extracts the X-Tenant-ID header
// from each request. If the header is missing or empty, it responds with
// 400 Bad Request and a JSON error body; otherwise it stores the tenant ID
// in the request context and delegates to next.
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			WriteError(w, http.StatusBadRequest, "missing or empty X-Tenant-ID header")
			return
		}

		ctx := context.WithValue(r.Context(), tenantKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantFrom extracts the tenant ID stored in the context by TenantMiddleware.
// The parameter is named r and is of type context.Context.
// Returns the tenant ID and true when present, or "" and false when absent.
func TenantFrom(r context.Context) (string, bool) {
	v, ok := r.Value(tenantKey).(string)
	return v, ok
}
