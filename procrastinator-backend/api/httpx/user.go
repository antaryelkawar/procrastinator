package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// UserMiddleware returns middleware that resolves the active user for a
// request. The user ID is read from the {userId} URL path parameter.
//
// Decision table: empty/missing → 400, malformed → 400, registry error → 500,
// unregistered → 404. Registered users are stored in ctx via user.WithUser.
func UserMiddleware(reg repo.UserRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "userId")
			if id == "" {
				WriteError(w, http.StatusBadRequest, "missing userId")
				return
			}
			if !user.Valid(id) {
				WriteError(w, http.StatusBadRequest, "malformed userId")
				return
			}
			registered, err := reg.Has(r.Context(), id)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "user lookup failed")
				return
			}
			if !registered {
				WriteError(w, http.StatusNotFound, "unknown user")
				return
			}
			ctx := user.WithUser(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
