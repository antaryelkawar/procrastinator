package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// unknownUUID is a fixed, well-formed UUID that never exists in any user.
const unknownUUID = "00000000-0000-4000-8000-000000000000"

// insertMovement inserts one manual movement directly via SQL and returns its
// id. source/dest are "" when the kind has no such account.
func insertMovement(t *testing.T, e *testEnv, OwnerID, kind, amount, currency, occurredOn, desc, source, dest string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var src, dst any
	if source != "" {
		src = source
	}
	if dest != "" {
		dst = dest
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, OwnerID); err != nil {
		t.Fatalf("set user: %v", err)
	}

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO money_movements (owner_id, kind, amount, currency, occurred_on, description, norm_description, origin, source_account_id, destination_account_id)
		 VALUES ($1, $2, $3::numeric, $4, $5::date, $6, $7, 'manual', $8, $9) RETURNING id`,
		OwnerID, kind, amount, currency, occurredOn, desc, strings.ToLower(desc), src, dst,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert movement %s: %v", kind, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

// TestCreateAccount exercises POST /api/finance/accounts.
func TestCreateAccount(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR","institution":"HDFC Bank","external_descriptor":"XX1234"}`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var acc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
			t.Fatalf("unmarshal account JSON: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(acc, "id"); got == "" {
			t.Error("id is empty, want non-empty")
		}
		if got := strVal(acc, "name"); got != "HDFC Savings" {
			t.Errorf("name = %q, want %q", got, "HDFC Savings")
		}
		if got := strVal(acc, "type"); got != "bank" {
			t.Errorf("type = %q, want %q", got, "bank")
		}
		if got := strVal(acc, "currency"); got != "INR" {
			t.Errorf("currency = %q, want %q", got, "INR")
		}
		if got := strVal(acc, "institution"); got != "HDFC Bank" {
			t.Errorf("institution = %q, want %q", got, "HDFC Bank")
		}
		if got := strVal(acc, "external_descriptor"); got != "XX1234" {
			t.Errorf("external_descriptor = %q, want %q", got, "XX1234")
		}
		if got := strVal(acc, "balance"); got != "0" {
			t.Errorf("balance = %q, want %q", got, "0")
		}
		if !strings.Contains(rec.Body.String(), `"balance":"0"`) {
			t.Errorf("raw body missing %q (balance must be a JSON string): %s", `"balance":"0"`, rec.Body.String())
		}
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"investment","currency":"INR"}`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		list := do(t, e.handler, http.MethodGet, "/api/finance/accounts", "test-user", nil, "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200 (body: %s)", list.Code, list.Body.String())
		}
		if got := strings.TrimSpace(list.Body.String()); got != "[]" {
			t.Fatalf("list body = %q, want exactly %q (nothing persisted)", got, "[]")
		}
	})

	t.Run("BlankName", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"","type":"bank","currency":"INR"}`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("InvalidCurrency", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"IN"}`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListAccounts exercises GET /api/finance/accounts.
func TestListAccounts(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/accounts", "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("WithData", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		creates := []string{
			`{"name":"HDFC Savings","type":"bank","currency":"INR"}`,
			`{"name":"Cash","type":"cash","currency":"INR"}`,
			`{"name":"Cash","type":"cash","currency":"INR"}`,
		}
		for _, body := range creates {
			rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", bytes.NewBufferString(body), "application/json")
			if rec.Code != http.StatusCreated {
				t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
			}
		}

		rec := do(t, e.handler, http.MethodGet, "/api/finance/accounts", "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var accs []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &accs); err != nil {
			t.Fatalf("unmarshal accounts: %v (body: %s)", err, rec.Body.String())
		}
		if len(accs) != 3 {
			t.Fatalf("account count = %d, want 3 (body: %s)", len(accs), rec.Body.String())
		}

		ids := make(map[string]bool)
		cashCount := 0
		for i, a := range accs {
			id := strVal(a, "id")
			if id == "" {
				t.Errorf("entry %d id is empty, want non-empty", i)
			}
			if ids[id] {
				t.Errorf("entry %d id %q is not distinct", i, id)
			}
			ids[id] = true
			if got := strVal(a, "balance"); got != "0" {
				t.Errorf("entry %d balance = %q, want %q", i, got, "0")
			}
			if strVal(a, "name") == "Cash" {
				cashCount++
			}
		}
		if cashCount != 2 {
			t.Errorf("Cash account count = %d, want 2 (duplicate names allowed): %s", cashCount, rec.Body.String())
		}
	})
}

// TestGetAccount exercises GET /api/finance/accounts/{id}.
func TestGetAccount(t *testing.T) {
	t.Parallel()

	t.Run("FoundWithDerivedBalance", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var created map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created account: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(created, "id")
		if id == "" {
			t.Fatalf("created account id is empty")
		}

		// Income of 250 into the account, expense of 100 out of it: derived
		// balance must be exactly "150".
		insertMovement(t, e, "test-user", "income", "250", "INR", "2026-08-01", "salary", "", id)
		insertMovement(t, e, "test-user", "expense", "100", "INR", "2026-08-02", "coffee", id, "")

		rec = do(t, e.handler, http.MethodGet, "/api/finance/accounts/"+id, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var acc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
			t.Fatalf("unmarshal account: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(acc, "balance"); got != "150" {
			t.Errorf("balance = %q, want %q (exact string)", got, "150")
		}
		if got := strVal(acc, "name"); got != "HDFC Savings" {
			t.Errorf("name = %q, want %q", got, "HDFC Savings")
		}
		if got := strVal(acc, "type"); got != "bank" {
			t.Errorf("type = %q, want %q", got, "bank")
		}
		if got := strVal(acc, "currency"); got != "INR" {
			t.Errorf("currency = %q, want %q", got, "INR")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/accounts/"+unknownUUID, "test-user", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("CrossUser", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var created map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created account: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(created, "id")
		if id == "" {
			t.Fatalf("created account id is empty")
		}

		rec = do(t, e.handler, http.MethodGet, "/api/finance/accounts/"+id, "test-user-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestBalanceCannotBeSetDirectly pins the derived-balance contract: a client
// can never set or modify an account's balance. Balance is computed from
// movements only; any "balance" field in a request body is ignored and no
// endpoint exists to write one.
func TestBalanceCannotBeSetDirectly(t *testing.T) {
	t.Parallel()

	t.Run("CreateIgnoresBalanceField", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR","balance":"999"}`)

		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var acc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
			t.Fatalf("unmarshal account JSON: %v (body: %s)", err, rec.Body.String())
		}
		// The supplied "balance":"999" must be dropped: balance is derived
		// from movements and is "0" for a fresh account.
		if got := strVal(acc, "balance"); got != "0" {
			t.Errorf("balance = %q, want %q (supplied balance field must be ignored)", got, "0")
		}
	})

	t.Run("NoBalanceUpdateRoute", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR"}`)
		rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", "test-user", body, "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var created map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created account: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(created, "id")
		if id == "" {
			t.Fatalf("created account id is empty")
		}

		// PUT /api/finance/accounts/{id} is not registered (GET only), so chi
		// rejects the method with 405 Method Not Allowed.
		putBody := bytes.NewBufferString(`{"balance":"500"}`)
		rec = do(t, e.handler, http.MethodPut, "/api/finance/accounts/"+id, "test-user", putBody, "application/json")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("PUT status = %d, want 405 (body: %s)", rec.Code, rec.Body.String())
		}

		// The derived balance must be unchanged by the rejected update.
		rec = do(t, e.handler, http.MethodGet, "/api/finance/accounts/"+id, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var acc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
			t.Fatalf("unmarshal account JSON: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(acc, "balance"); got != "0" {
			t.Errorf("balance = %q, want %q (derived balance must be unchanged)", got, "0")
		}
	})
}

// TestAccountsMissingUser exercises the user-identity contract: every account
// endpoint with an empty {userId} path segment is rejected with 400.
func TestAccountsMissingUser(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	body := bytes.NewBufferString(`{"name":"HDFC Savings","type":"bank","currency":"INR"}`)

	cases := []struct {
		name   string
		method string
		path   string
		body   *bytes.Buffer
		cType  string
	}{
		{name: "POST /api/users//finance/accounts", method: http.MethodPost, path: "/api/users//finance/accounts", body: body, cType: "application/json"},
		{name: "GET /api/users//finance/accounts", method: http.MethodGet, path: "/api/users//finance/accounts"},
		{name: "GET /api/users//finance/accounts/{id}", method: http.MethodGet, path: "/api/users//finance/accounts/" + unknownUUID},
	}
	// The user middleware rejects an empty {userId} path segment with 400.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := do(t, e.handler, tc.method, tc.path, "", tc.body, tc.cType)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
			assertErrorEnvelope(t, rec)
		})
	}
}
