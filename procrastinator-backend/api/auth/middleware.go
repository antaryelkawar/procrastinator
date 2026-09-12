// Package auth provides the Basic Auth HTTP middleware for the procrastinator
// backend. It validates the request's Authorization header against a set of
// configured credential pairs using constant-time comparison so that the
// response (and its timing) does not reveal which configured user or password
// matched, which character mismatched, or which pair was reached.
package auth

import (
	"crypto/subtle"
	"net/http"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/config"
)

// maxUserLen and maxPassLen are the fixed byte-length buffers used for
// constant-time comparison (design decision D2). The config schema allows
// user 1-64 UTF-8 runes and pass 8-128 UTF-8 runes; each rune is at most 4
// bytes, so 64×4=256 and 128×4=512 are the worst-case byte lengths.
const (
	maxUserLen = 256
	maxPassLen = 512
)

// Middleware returns an HTTP middleware that enforces HTTP Basic Auth against
// the configured credential pairs.
//
// The comparison is constant-time: for each configured pair, the client's
// user and pass are zero-padded to a fixed byte length and compared with
// subtle.ConstantTimeCompare, then the per-field results are ANDed and the
// per-pair result is OR-accumulated across all pairs without short-circuiting.
// This prevents an attacker from inferring which pair matched or which
// character/field mismatched from timing or response differences.
//
// Padding is required because subtle.ConstantTimeCompare short-circuits (and
// returns 0) when the two inputs have different lengths, which would leak
// length information about the configured secrets.
//
// Rejections set the WWW-Authenticate: Basic header and write a 401 with the
// standard {"error": "invalid credentials"} JSON envelope via httpx.
func Middleware(pairs []config.Credential) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if ok {
				matched := 0
				for _, p := range pairs {
					uOK := padCompare(user, p.User, maxUserLen)
					pOK := padCompare(pass, p.Pass, maxPassLen)
					matched |= uOK & pOK
				}
				if matched != 0 {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.Header().Set("WWW-Authenticate", "Basic")
			httpx.WriteErrorEnvelope(w, http.StatusUnauthorized, "invalid credentials")
		})
	}
}

// padCompare zero-pads a and b to length n bytes (copying each, truncating if
// longer) and returns the result of subtle.ConstantTimeCompare (0 or 1).
// Copying into a fixed-size buffer ensures both inputs to the comparison are
// always exactly n bytes long, so no length information leaks.
func padCompare(a, b string, n int) int {
	bufA := make([]byte, n)
	bufB := make([]byte, n)
	copy(bufA, a)
	copy(bufB, b)
	return subtle.ConstantTimeCompare(bufA, bufB)
}
