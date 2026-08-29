package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/commons/tenant"
)

// fakeRegistry is an in-memory implementation of repo.TenantRegistry for tests.
type fakeRegistry struct {
	tenants map[string]bool
	err     error
}

func (f *fakeRegistry) Has(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.tenants[id], nil
}

// requestWithUserID builds a request whose context carries the {userId} route
// param, mimicking the context chi produces after matching
// /api/users/{userId}/....
func requestWithUserID(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userId", userID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// runMiddleware runs the middleware around a handler that records whether it
// was called and returns the recorded response plus the context the handler
// saw.
func runMiddleware(t *testing.T, reg *fakeRegistry, userID string) (*httptest.ResponseRecorder, bool, context.Context) {
	t.Helper()

	var called bool
	var gotCtx context.Context
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotCtx = r.Context()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mw := TenantMiddleware(reg)(inner)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, requestWithUserID(userID))
	return rec, called, gotCtx
}

func TestTenantMiddleware_Missing(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{tenants: map[string]bool{"acme": true}}
	rec, called, _ := runMiddleware(t, reg, "")

	if called {
		t.Error("handler was called, want not called")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "missing") {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), "missing")
	}
}

func TestTenantMiddleware_Malformed(t *testing.T) {
	t.Parallel()

	longID := strings.Repeat("a", 65)
	cases := []struct {
		name   string
		userID string
	}{
		{name: "65 characters", userID: longID},
		{name: "space", userID: "a b"},
		{name: "slash", userID: "a/b"},
		{name: "semicolon", userID: "a;b"},
		{name: "leading space", userID: " a"},
		{name: "trailing space", userID: "a "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reg := &fakeRegistry{tenants: map[string]bool{}}
			rec, called, _ := runMiddleware(t, reg, tc.userID)

			if called {
				t.Error("handler was called, want not called")
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), "malformed") {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), "malformed")
			}
		})
	}
}

func TestTenantMiddleware_Unregistered(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{tenants: map[string]bool{"acme": true}}
	rec, called, _ := runMiddleware(t, reg, "unknown")

	if called {
		t.Error("handler was called, want not called")
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "unknown user") {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), "unknown user")
	}
}

func TestTenantMiddleware_RegistryError(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{
		tenants: map[string]bool{"acme": true},
		err:     errors.New("db down"),
	}
	rec, called, _ := runMiddleware(t, reg, "acme")

	if called {
		t.Error("handler was called, want not called")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "tenant lookup failed") {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), "tenant lookup failed")
	}
}

func TestTenantMiddleware_Registered(t *testing.T) {
	t.Parallel()

	reg := &fakeRegistry{tenants: map[string]bool{"acme": true}}
	rec, called, gotCtx := runMiddleware(t, reg, "acme")

	if !called {
		t.Fatal("handler was not called, want called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}

	id, err := tenant.TenantFrom(gotCtx)
	if err != nil {
		t.Fatalf("tenant.TenantFrom = %v, want no error", err)
	}
	if id != "acme" {
		t.Errorf("tenant = %q, want %q", id, "acme")
	}
}
