package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/ledger"
)

// wireUserReg is the user registry for the wire-parity test: "test-user" is
// registered (so UserMiddleware passes and the handler runs), everything else
// is unregistered (404 {"error":"unknown user"}).
type wireUserReg struct{}

func (wireUserReg) Has(_ context.Context, id string) (bool, error) {
	return id == "test-user", nil
}

var _ repo.UserRegistry = wireUserReg{}

// wireAssetRepo is an in-memory repo.Repository[entity.Asset] for the
// wire-parity test: List returns a fixed (non-nil) list and every other
// verb is an error stub (this test never reaches them).
type wireAssetRepo struct {
	list []entity.Asset
}

func (r wireAssetRepo) List(_ context.Context, _ ...repo.Option) ([]entity.Asset, error) {
	if r.list != nil {
		return r.list, nil
	}
	return []entity.Asset{}, nil
}

func (wireAssetRepo) Get(_ context.Context, _ string, _ ...repo.Option) (entity.Asset, error) {
	return entity.Asset{}, repo.ErrNotFound
}

func (wireAssetRepo) Create(_ context.Context, _ entity.Asset, _ ...repo.Option) (entity.Asset, error) {
	return entity.Asset{}, errors.New("wireAssetRepo: create not supported")
}

func (wireAssetRepo) Update(_ context.Context, _ entity.Asset, _ ...repo.Option) (entity.Asset, error) {
	return entity.Asset{}, errors.New("wireAssetRepo: update not supported")
}

func (wireAssetRepo) Delete(_ context.Context, _ string, _ ...repo.Option) error {
	return errors.New("wireAssetRepo: delete not supported")
}

var _ repo.Repository[entity.Asset] = wireAssetRepo{}

// wireMovementRepo is an in-memory repo.Repository[entity.MoneyMovement] for
// the wire-parity test: Get returns the single fixed movement (whose
// LinkedDocumentID is nil, so the ledger service's Unlink is a no-op), List
// returns a non-nil empty list, and Create/Delete are error stubs.
type wireMovementRepo struct {
	mv entity.MoneyMovement
}

func (r wireMovementRepo) Get(_ context.Context, _ string, _ ...repo.Option) (entity.MoneyMovement, error) {
	return r.mv, nil
}

func (wireMovementRepo) List(_ context.Context, _ ...repo.Option) ([]entity.MoneyMovement, error) {
	return []entity.MoneyMovement{}, nil
}

func (wireMovementRepo) Create(_ context.Context, _ entity.MoneyMovement, _ ...repo.Option) (entity.MoneyMovement, error) {
	return entity.MoneyMovement{}, errors.New("wireMovementRepo: create not supported")
}

func (r wireMovementRepo) Update(_ context.Context, mv entity.MoneyMovement, _ ...repo.Option) (entity.MoneyMovement, error) {
	return mv, nil
}

func (wireMovementRepo) Delete(_ context.Context, _ string, _ ...repo.Option) error {
	return errors.New("wireMovementRepo: delete not supported")
}

var _ repo.Repository[entity.MoneyMovement] = wireMovementRepo{}

// wireBalancer is an in-memory ledger.BalanceQuerier for the wire-parity
// test: it reports a derived balance of "0" for every account.
type wireBalancer struct{}

func (wireBalancer) BalanceForAccount(_ context.Context, _ string, _ ...repo.Option) (string, error) {
	return "0", nil
}

var _ ledger.BalanceQuerier = wireBalancer{}

// TestWireParity asserts the documented wire contract of the strict-server
// transport end-to-end through (*Server).Routes(): the same set of requests
// produces the documented status, Content-Type, and body shape — the
// {"error": string} envelope for pre-handler and binding failures, the
// generated JSON for success, and no body for 204. The generated encoder
// appends a trailing "\n" to JSON success bodies (benign, documented); the
// assertions tolerate that.
//
// The stack is the real generated strict server + centralized error funcs +
// UserMiddleware + MaxBodyMiddleware, exactly as production wires it (the
// ingest/ledger services are nil because every case in this test resolves
// before any service call: middleware, binding, or a registry lookup that
// returns not-found).
func TestWireParity(t *testing.T) {
	t.Parallel()

	factory := &repo.Factory{
		Users:     wireUserReg{},
		Assets:    wireAssetRepo{list: []entity.Asset{{ID: "asset-1", DocType: "invoice"}}},
		Movements: wireMovementRepo{mv: entity.MoneyMovement{ID: "mv-1"}},
	}
	s := New(nil, factory, ledger.New(factory, wireBalancer{}), wireBalancer{}, defaultMaxBytes, nil, defaultMaxBytes, nil)
	handler := s.Routes()

	do := func(method, path, body string, ct string) *httptest.ResponseRecorder {
		t.Helper()
		var reader interface{ Read([]byte) (int, error) }
		if body != "" {
			reader = bytes.NewBufferString(body)
		}
		req := httptest.NewRequest(method, path, reader)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	assertStatus := func(rec *httptest.ResponseRecorder, wantStatus int) {
		t.Helper()
		if rec.Code != wantStatus {
			t.Errorf("status = %d, want %d (body: %s)", rec.Code, wantStatus, rec.Body.String())
		}
	}

	assertJSON := func(rec *httptest.ResponseRecorder, wantStatus int) {
		t.Helper()
		assertStatus(rec, wantStatus)
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json (status %d)", ct, rec.Code)
		}
	}

	assertEnvelope := func(rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
		t.Helper()
		assertJSON(rec, wantStatus)
		if wantMsg != "" && !strings.Contains(rec.Body.String(), wantMsg) {
			t.Errorf("body = %q, want to contain error %q", rec.Body.String(), wantMsg)
		}
	}

	const unknownID = "00000000-0000-4000-8000-000000000000"

	t.Run("UnknownUserEnvelope", func(t *testing.T) {
		t.Parallel()
		// "unknown-user" is unregistered in wireUserReg (which registers only
		// test-user), so the UserMiddleware's pre-handler path returns 404
		// {"error":"unknown user"} before the strict handler runs.
		rec := do(http.MethodGet, "/api/users/unknown-user/assets", "", "")
		assertEnvelope(rec, http.StatusNotFound, "unknown user")
	})

	t.Run("NotFoundRoutePlain404", func(t *testing.T) {
		t.Parallel()
		// An unroutable path (no registered route matches) falls through to
		// chi's default plain-text 404 — NOT the JSON envelope. This preserves
		// the base change's wire contract for unroutable paths; the JSON
		// {"error": string} envelope is reserved for resolved-handler and
		// middleware failures.
		rec := do(http.MethodGet, "/api/users/test-user/nope", "", "")
		assertStatus(rec, http.StatusNotFound)
		if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want no application/json (plain-text chi 404)", ct)
		}
	})

	t.Run("MalformedJSONEnvelope", func(t *testing.T) {
		t.Parallel()
		// Malformed JSON on a JSON-body op surfaces as 400
		// {"error":"malformed request"} from the centralized request error func
		// (the generated strict handler would otherwise emit a plain-text 400).
		rec := do(http.MethodPost, "/api/users/test-user/finance/accounts", `{"name":`, "application/json")
		assertEnvelope(rec, http.StatusBadRequest, "malformed request")
	})

	t.Run("SuccessListJSON", func(t *testing.T) {
		t.Parallel()
		// A resolved list op runs through the generated strict handler and the
		// centralized success encoder: 200 with a generated JSON array body.
		rec := do(http.MethodGet, "/api/users/test-user/assets", "", "")
		assertJSON(rec, http.StatusOK)
		if body := strings.TrimSpace(rec.Body.String()); !strings.HasPrefix(body, "[") {
			t.Errorf("body = %q, want a JSON array", body)
		}
		if !strings.Contains(rec.Body.String(), `"id":"asset-1"`) {
			t.Errorf("body = %q, want to contain %q", rec.Body.String(), `"id":"asset-1"`)
		}
	})

	t.Run("NoContentEmptyBody", func(t *testing.T) {
		t.Parallel()
		// A no-content op (unlink of an already-unlinked movement) returns 204
		// with an empty body.
		rec := do(http.MethodDelete, "/api/users/test-user/finance/movements/mv-1/link", "", "")
		assertStatus(rec, http.StatusNoContent)
		if got := rec.Body.String(); got != "" {
			t.Errorf("body = %q, want empty", got)
		}
	})
}
