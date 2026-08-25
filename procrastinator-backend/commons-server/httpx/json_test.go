package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

type note struct {
	Message string `json:"message"`
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     int
		payload    any
		wantStatus int
		wantBody   string
	}{
		{
			name:       "status 200 with map payload",
			status:     200,
			payload:    map[string]string{"status": "ok"},
			wantStatus: 200,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "status 201 with struct payload",
			status:     201,
			payload:    note{Message: "created"},
			wantStatus: 201,
			wantBody:   `{"message":"created"}`,
		},
		{
			name:       "status 404 with zero struct value",
			status:     404,
			payload:    note{},
			wantStatus: 404,
			wantBody:   `{"message":""}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			WriteJSON(rec, tc.status, tc.payload)

			res := rec.Result()
			if res.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want to contain 'application/json'", ct)
			}

			body := rec.Body.String()
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     int
		msg        string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "status 401 missing tenant header",
			status:     401,
			msg:        "missing or empty X-Tenant-ID header",
			wantStatus: 401,
			wantBody:   `{"error":"missing or empty X-Tenant-ID header"}`,
		},
		{
			name:       "status 400 bad input",
			status:     400,
			msg:        "bad input",
			wantStatus: 400,
			wantBody:   `{"error":"bad input"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			WriteError(rec, tc.status, tc.msg)

			res := rec.Result()
			if res.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want to contain 'application/json'", ct)
			}

			body := rec.Body.String()
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
