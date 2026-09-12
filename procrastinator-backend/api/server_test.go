package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/config"
	"procrastinator-backend/internal/testcred"
)

// Integration tests for the Basic Auth wiring in (*Server).Routes(): the
// auth middleware must be the OUTERMOST layer of the generated middleware
// chain (design D3), so it rejects unauthenticated requests before
// UserMiddleware consults the user registry.
//
// These tests are DB-independent: they build the Server with a fake
// repo.Factory (like wire_parity_test.go) and drive the real Routes() chain.

// testBasicAuthCreds is the shared fixture credential set for the auth-wiring
// tests (internal/testcred — same set the api/e2e integration helpers send).
var testBasicAuthCreds = testcred.Creds

// basicAuthHeader builds the Authorization header value for a credential pair.
func basicAuthHeader(user, pass string) string {
	return testcred.HeaderFor(config.Credential{User: user, Pass: pass})
}

// countingUserReg is a repo.UserRegistry fake that records every lookup so
// tests can pin the middleware order: auth must reject before the registry
// is consulted. "test-user" is the only registered id.
type countingUserReg struct {
	mu      sync.Mutex
	lookups int
}

func (c *countingUserReg) Has(_ context.Context, id string) (bool, error) {
	c.mu.Lock()
	c.lookups++
	c.mu.Unlock()
	return id == "test-user", nil
}

func (c *countingUserReg) lookupCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lookups
}

var _ repo.UserRegistry = (*countingUserReg)(nil)

// TestNewStoresBasicAuthUsers pins the constructor contract (task 2.3): New
// stores the []config.Credential argument on the Server so Routes() can
// enforce Basic Auth from it. A nil slice is stored as-is (reject-everything
// semantics documented on the field).
func TestNewStoresBasicAuthUsers(t *testing.T) {
	t.Parallel()

	creds := []config.Credential{{User: "inject-user", Pass: "inject-password-1"}}
	if got := New(nil, nil, nil, nil, defaultMaxBytes, nil, defaultMaxBytes, nil, nil, nil, nil, creds).basicAuthUsers; len(got) != 1 || got[0] != creds[0] {
		t.Errorf("basicAuthUsers = %+v, want the exact credential pair passed to New", got)
	}
	if got := New(nil, nil, nil, nil, defaultMaxBytes, nil, defaultMaxBytes, nil, nil, nil, nil, nil).basicAuthUsers; got != nil {
		t.Errorf("basicAuthUsers = %+v, want nil when nil is passed", got)
	}
}

// newAuthTestHandler builds the real Routes() chain over a fake factory whose
// user registry counts lookups. Assets list returns one fixed asset so the
// authenticated happy path reaches the handler and encodes 200.
func newAuthTestHandler() (http.Handler, *countingUserReg) {
	reg := &countingUserReg{}
	factory := &repo.Factory{
		Users:     reg,
		Assets:    wireAssetRepo{list: []entity.Asset{{ID: "asset-1"}}},
		Movements: wireMovementRepo{mv: entity.MoneyMovement{ID: "mv-1"}},
	}
	s := New(nil, factory, nil, nil, defaultMaxBytes, nil, defaultMaxBytes, nil, nil, nil, nil, testBasicAuthCreds)
	return s.Routes(), reg
}

func getWithAuth(t *testing.T, handler http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestRoutesBasicAuth pins the auth middleware's behavior and position in the
// Routes() chain: 401-without-creds, 401-on-registered-user-path-with-bad-creds
// with ZERO registry lookups (auth runs outermost, before tenancy resolution),
// and 200 on valid creds.
func TestRoutesBasicAuth(t *testing.T) {
	t.Parallel()

	const registeredUserPath = "/api/users/test-user/assets"

	t.Run("RejectsWithoutCredentials", func(t *testing.T) {
		t.Parallel()
		handler, _ := newAuthTestHandler()
		rec := getWithAuth(t, handler, registeredUserPath, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != "Basic" {
			t.Errorf("WWW-Authenticate = %q, want %q", got, "Basic")
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if body := rec.Body.String(); !strings.Contains(body, "invalid credentials") {
			t.Errorf("body = %q, want the {\"error\": string} envelope naming invalid credentials", body)
		}
	})

	t.Run("RejectsBadCredsBeforeRegistryLookup", func(t *testing.T) {
		t.Parallel()
		// Order pin (design D3): "test-user" IS registered in the fake, so if
		// UserMiddleware ran first this request would pass tenancy and reach
		// the handler (200) or at least record a lookup. A 401 with zero
		// lookups proves auth wrapped outermost and short-circuited the chain.
		handler, reg := newAuthTestHandler()
		rec := getWithAuth(t, handler, registeredUserPath, basicAuthHeader("fixture-user", "wrong-password"))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
		}
		if n := reg.lookupCount(); n != 0 {
			t.Errorf("registry lookups = %d, want 0: auth must run before tenancy resolution", n)
		}
	})

	t.Run("RejectsWrongUserBeforeRegistryLookup", func(t *testing.T) {
		t.Parallel()
		handler, reg := newAuthTestHandler()
		rec := getWithAuth(t, handler, registeredUserPath, basicAuthHeader("other-user", "fixture-password-1"))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
		}
		if n := reg.lookupCount(); n != 0 {
			t.Errorf("registry lookups = %d, want 0: auth must run before tenancy resolution", n)
		}
	})

	t.Run("RejectsNonBasicScheme", func(t *testing.T) {
		t.Parallel()
		handler, reg := newAuthTestHandler()
		rec := getWithAuth(t, handler, registeredUserPath, "Bearer sometoken")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
		}
		if n := reg.lookupCount(); n != 0 {
			t.Errorf("registry lookups = %d, want 0", n)
		}
	})

	t.Run("AcceptsValidCredentials", func(t *testing.T) {
		t.Parallel()
		handler, reg := newAuthTestHandler()
		rec := getWithAuth(t, handler, registeredUserPath, basicAuthHeader("fixture-user", "fixture-password-1"))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if body := rec.Body.String(); !strings.Contains(body, `"asset-1"`) {
			t.Errorf("body = %q, want the fake asset list from the handler", body)
		}
		// Valid creds must pass through to tenancy: exactly one registry lookup.
		if n := reg.lookupCount(); n != 1 {
			t.Errorf("registry lookups = %d, want 1: the request should reach UserMiddleware", n)
		}
	})
}
