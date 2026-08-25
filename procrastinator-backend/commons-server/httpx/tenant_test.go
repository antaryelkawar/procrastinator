package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTenantMiddleware(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		tenantID    string
		hasHeader   bool
		wantStatus  int
		wantBody    string
		wantHandled bool
		wantTenant  string
	}{
		{
			name:        "valid X-Tenant-ID header",
			tenantID:    "test-tenant",
			hasHeader:   true,
			wantStatus:  200,
			wantBody:    "ok",
			wantHandled: true,
			wantTenant:  "test-tenant",
		},
		{
			name:        "missing X-Tenant-ID header",
			tenantID:    "",
			hasHeader:   false,
			wantStatus:  400,
			wantBody:    `{"error":"missing or empty X-Tenant-ID header"}`,
			wantHandled: false,
			wantTenant:  "",
		},
		{
			name:        "empty X-Tenant-ID header",
			tenantID:    "",
			hasHeader:   true,
			wantStatus:  400,
			wantBody:    `{"error":"missing or empty X-Tenant-ID header"}`,
			wantHandled: false,
			wantTenant:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handlerCalled := false
			innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				tenant, ok := TenantFrom(r.Context())
				if !ok || tenant != tc.wantTenant {
					t.Errorf("TenantFrom returned (%q, %v), want (%q, true)", tenant, ok, tc.wantTenant)
				}
				w.WriteHeader(200)
				w.Write([]byte("ok"))
			})

			mw := TenantMiddleware(innerHandler)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			if tc.hasHeader {
				req.Header.Set("X-Tenant-ID", tc.tenantID)
			}

			mw.ServeHTTP(rec, req)

			if handlerCalled != tc.wantHandled {
				t.Errorf("handlerCalled = %v, want %v", handlerCalled, tc.wantHandled)
			}

			res := rec.Result()
			if res.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}

			if tc.wantStatus == 400 {
				ct := res.Header.Get("Content-Type")
				if !strings.Contains(ct, "application/json") {
					t.Errorf("Content-Type = %q, want to contain 'application/json'", ct)
				}
			}

			body := rec.Body.String()
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestTenantFromAbsent(t *testing.T) {
	t.Parallel()
	tenant, ok := TenantFrom(context.Background())
	if ok || tenant != "" {
		t.Errorf("TenantFrom(context.Background()) = (%q, %v), want (\"\", false)", tenant, ok)
	}
}
