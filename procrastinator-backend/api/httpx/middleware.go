package httpx

import (
	"net/http"
	"strings"
)

// MaxBodyMiddleware returns a chi middleware that wraps the request body with
// http.MaxBytesReader for the two upload routes. It must be applied at the chi
// level (not as a strict middleware) because the strict handler creates the
// multipart reader before any strict middleware runs, so the body size limit
// must be installed where w/r are available and the concrete path is present.
//
// Matching is by method + concrete path, since the request path carries the
// real user ID rather than the {userId} pattern:
//
//   - POST /api/users/{userId}/documents             -> documentLimit
//   - POST /api/users/{userId}/finance/import-batches -> statementLimit
//
// Any other method or path is passed through unchanged.
func MaxBodyMiddleware(
	documentLimit int64,
	statementLimit int64,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				switch {
				case isDocumentUpload(r.URL.Path):
					r.Body = http.MaxBytesReader(w, r.Body, documentLimit)
				case isStatementUpload(r.URL.Path):
					r.Body = http.MaxBytesReader(w, r.Body, statementLimit)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isDocumentUpload reports whether the path matches
// /api/users/{userId}/documents.
func isDocumentUpload(path string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// Expected: ["api", "users", "{userId}", "documents"]
	return len(segments) == 4 &&
		segments[0] == "api" &&
		segments[1] == "users" &&
		segments[3] == "documents"
}

// isStatementUpload reports whether the path matches
// /api/users/{userId}/finance/import-batches.
func isStatementUpload(path string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// Expected: ["api", "users", "{userId}", "finance", "import-batches"]
	return len(segments) == 5 &&
		segments[0] == "api" &&
		segments[1] == "users" &&
		segments[3] == "finance" &&
		segments[4] == "import-batches"
}
