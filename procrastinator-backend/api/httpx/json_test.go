package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorEnvelope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     int
		msg        string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "status 401 missing user identity",
			status:     http.StatusUnauthorized,
			msg:        "missing or invalid user identity",
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"missing or invalid user identity"}`,
		},
		{
			name:       "status 400 bad input",
			status:     http.StatusBadRequest,
			msg:        "bad input",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"bad input"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			WriteErrorEnvelope(rec, tc.status, tc.msg)

			res := rec.Result()
			if res.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}
			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want to contain 'application/json'", ct)
			}
			if body := rec.Body.String(); body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
