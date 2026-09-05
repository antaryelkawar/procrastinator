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
//
// It is installed as a chi-level middleware (r.Use / chi ServerOptions
// middlewares) so it runs BEFORE the generated strict handler would try to
// bind an empty {userId} (the generated binding would emit a plain-text 400
// "Invalid format for parameter userId").
func UserMiddleware(reg repo.UserRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "userId")
			if id == "" {
				WriteErrorEnvelope(w, http.StatusBadRequest, "missing userId")
				return
			}
			if !user.Valid(id) {
				WriteErrorEnvelope(w, http.StatusBadRequest, "malformed userId")
				return
			}
			registered, err := reg.Has(r.Context(), id)
			if err != nil {
				WriteErrorEnvelope(w, http.StatusInternalServerError, "user lookup failed")
				return
			}
			if !registered {
				WriteErrorEnvelope(w, http.StatusNotFound, "unknown user")
				return
			}
			ctx := user.WithUser(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
