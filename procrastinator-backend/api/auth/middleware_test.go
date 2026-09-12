package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"procrastinator-backend/config"
)

// testPairs mirrors the real config bounds (user 1-64 runes, pass 8-128 runes).
var testPairs = []config.Credential{
	{User: "app", Pass: "s3cret1!"},
	{User: "app2", Pass: "anotherpass"},
}

// runBasic runs Middleware(testPairs) around a sentinel next handler and
// returns the recorder plus whether next was called.
func runBasic(t *testing.T, authHeader string) (*httptest.ResponseRecorder, bool) {
	t.Helper()

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	Middleware(testPairs)(next).ServeHTTP(rec, req)
	return rec, called
}

// rejectionTriple returns the observable rejection triple: status code,
// WWW-Authenticate header value, and full body.
func rejectionTriple(t *testing.T, authHeader string) (int, string, string) {
	t.Helper()
	rec, _ := runBasic(t, authHeader)
	return rec.Code, rec.Header().Get("WWW-Authenticate"), rec.Body.String()
}

// basicAuth builds the Authorization header value for user:pass.
func basicAuth(userPass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(userPass))
}

func TestMiddleware_Valid(t *testing.T) {
	t.Parallel()

	rec, called := runBasic(t, basicAuth("app:s3cret1!"))

	if !called {
		t.Fatal("handler was not called, want called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestMiddleware_ValidSecondPair(t *testing.T) {
	t.Parallel()

	rec, called := runBasic(t, basicAuth("app2:anotherpass"))

	if !called {
		t.Fatal("handler was not called, want called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	t.Parallel()

	rec, called := runBasic(t, "")

	if called {
		t.Error("handler was called, want not called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Basic" {
		t.Errorf("WWW-Authenticate = %q, want %q", got, "Basic")
	}
	if body := rec.Body.String(); body != `{"error":"invalid credentials"}` {
		t.Errorf("body = %q, want %q", body, `{"error":"invalid credentials"}`)
	}
}

func TestMiddleware_NonBasicScheme(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		hdr  string
	}{
		{name: "Bearer", hdr: "Bearer abc123"},
		{name: "Digest", hdr: "Digest abc123"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec, called := runBasic(t, tc.hdr)

			if called {
				t.Error("handler was called, want not called")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != "Basic" {
				t.Errorf("WWW-Authenticate = %q, want %q", got, "Basic")
			}
			if body := rec.Body.String(); body != `{"error":"invalid credentials"}` {
				t.Errorf("body = %q, want %q", body, `{"error":"invalid credentials"}`)
			}
		})
	}
}

func TestMiddleware_MalformedToken(t *testing.T) {
	t.Parallel()

	// Malformed Basic tokens that fail parsing in (*http.Request).BasicAuth
	// (invalid base64, or a decoded token with no ":" separator) must be
	// rejected identically to every other auth failure.
	cases := []struct {
		name string
		hdr  string
	}{
		{name: "invalid base64", hdr: "Basic not!valid@base64"},
		{name: "no colon separator", hdr: basicAuth("usersonly")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec, called := runBasic(t, tc.hdr)

			if called {
				t.Error("handler was called, want not called")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != "Basic" {
				t.Errorf("WWW-Authenticate = %q, want %q", got, "Basic")
			}
			if body := rec.Body.String(); body != `{"error":"invalid credentials"}` {
				t.Errorf("body = %q, want %q", body, `{"error":"invalid credentials"}`)
			}
		})
	}
}

func TestMiddleware_WrongCreds(t *testing.T) {
	t.Parallel()

	// valid base64, well-formed user:pass, not a configured pair
	rec, called := runBasic(t, basicAuth("app:wrongpass"))

	if called {
		t.Error("handler was called, want not called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Basic" {
		t.Errorf("WWW-Authenticate = %q, want %q", got, "Basic")
	}
	if body := rec.Body.String(); body != `{"error":"invalid credentials"}` {
		t.Errorf("body = %q, want %q", body, `{"error":"invalid credentials"}`)
	}
}

func TestMiddleware_RejectionParity(t *testing.T) {
	t.Parallel()

	// All variants are the same length as the valid credential
	// ("app:s3cret1!", 12 chars) and each is wrong in a different way: a
	// different character, a different field, or a cross-pair user/pass mix.
	// Equal length isolates the character/field difference from any length-based
	// variation in the comparison.
	variants := []struct {
		name     string
		userPass string
	}{
		{"wrong last char of pass", "app:s3cret1x"},
		{"swapped last two pass chars", "app:s3cret!1"},
		{"wrong user field", "apx:s3cret1!"},
		{"cross-pair: pair2 user + pair1 pass (minus last char)", "app2:s3cret1"},
	}

	refStatus, refWWW, refBody := rejectionTriple(t, variants[0].userPass)

	if refStatus != http.StatusUnauthorized {
		t.Fatalf("reference status = %d, want %d", refStatus, http.StatusUnauthorized)
	}
	if refWWW != "Basic" {
		t.Fatalf("reference WWW-Authenticate = %q, want %q", refWWW, "Basic")
	}

	for _, v := range variants[1:] {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotWWW, gotBody := rejectionTriple(t, v.userPass)
			if gotStatus != refStatus {
				t.Errorf("status = %d, want %d (parity)", gotStatus, refStatus)
			}
			if gotWWW != refWWW {
				t.Errorf("WWW-Authenticate = %q, want %q (parity)", gotWWW, refWWW)
			}
			if gotBody != refBody {
				t.Errorf("body = %q, want %q (parity)", gotBody, refBody)
			}
		})
	}
}
