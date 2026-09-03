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

// createAccount posts a finance account and returns its id.
func createAccount(t *testing.T, e *testEnv, userID, name, typ, currency string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"name":"` + name + `","type":"` + typ + `","currency":"` + currency + `"}`)
	rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", userID, body, "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account %q status = %d, want 201 (body: %s)", name, rec.Code, rec.Body.String())
	}
	var acc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
		t.Fatalf("unmarshal created account: %v (body: %s)", err, rec.Body.String())
	}
	id := strVal(acc, "id")
	if id == "" {
		t.Fatalf("created account id is empty")
	}
	return id
}

// getBalance returns the derived balance of accountID as an exact-decimal string.
func getBalance(t *testing.T, e *testEnv, userID, accountID string) string {
	t.Helper()
	rec := do(t, e.handler, http.MethodGet, "/api/finance/accounts/"+accountID, userID, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get account status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var acc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &acc); err != nil {
		t.Fatalf("unmarshal account: %v (body: %s)", err, rec.Body.String())
	}
	return strVal(acc, "balance")
}

// TestCreateMovement exercises POST /api/finance/movements.
func TestCreateMovement(t *testing.T) {
	t.Parallel()

	t.Run("ExpenseHappyPath", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "HDFC", "bank", "INR")

		body := `{"kind":"expense","amount":"1250.50","currency":"INR","occurred_on":"2026-08-20","description":"Reliance Digital","source_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal movement: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(mv, "origin"); got != "manual" {
			t.Errorf("origin = %q, want %q", got, "manual")
		}
		if got := strVal(mv, "kind"); got != "expense" {
			t.Errorf("kind = %q, want %q", got, "expense")
		}
		if got := strVal(mv, "occurred_on"); got != "2026-08-20" {
			t.Errorf("occurred_on = %q, want %q", got, "2026-08-20")
		}
		if got := strVal(mv, "source_account_id"); got != a {
			t.Errorf("source_account_id = %q, want %q", got, a)
		}
		if _, ok := mv["destination_account_id"]; ok {
			t.Errorf("destination_account_id present, want absent: %s", rec.Body.String())
		}
		for _, k := range []string{"import_batch_id", "import_line", "external_reference"} {
			if _, ok := mv[k]; ok {
				t.Errorf("key %q present for manual movement, want absent: %s", k, rec.Body.String())
			}
		}
		if got := strVal(mv, "id"); got == "" {
			t.Error("id is empty, want non-empty")
		}
		if !strings.Contains(rec.Body.String(), `"amount":"1250.50"`) {
			t.Errorf("raw body missing %q (amount must be a JSON string): %s", `"amount":"1250.50"`, rec.Body.String())
		}

		if bal := getBalance(t, e, "test-user", a); bal != "-1250.50" {
			t.Errorf("balance = %q, want %q", bal, "-1250.50")
		}
	})

	t.Run("IncomeHappyPath", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "HDFC", "bank", "INR")

		body := `{"kind":"income","amount":"75000","currency":"INR","occurred_on":"2026-08-20","description":"salary","destination_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal movement: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(mv, "destination_account_id"); got != a {
			t.Errorf("destination_account_id = %q, want %q", got, a)
		}
		if _, ok := mv["source_account_id"]; ok {
			t.Errorf("source_account_id present for income, want absent: %s", rec.Body.String())
		}

		if bal := getBalance(t, e, "test-user", a); bal != "75000" {
			t.Errorf("balance = %q, want %q", bal, "75000")
		}
	})

	t.Run("Transfer", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		b := createAccount(t, e, "test-user", "B", "cash", "INR")

		body := `{"kind":"transfer","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"move","source_account_id":"` + a + `","destination_account_id":"` + b + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		if bal := getBalance(t, e, "test-user", a); bal != "-100" {
			t.Errorf("balance A = %q, want %q", bal, "-100")
		}
		if bal := getBalance(t, e, "test-user", b); bal != "100" {
			t.Errorf("balance B = %q, want %q", bal, "100")
		}
	})

	t.Run("UnknownAccount", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		body := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + unknownUUID + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		list := do(t, e.handler, http.MethodGet, "/api/finance/movements", "test-user", nil, "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200 (body: %s)", list.Code, list.Body.String())
		}
		if got := strings.TrimSpace(list.Body.String()); got != "[]" {
			t.Fatalf("list body = %q, want exactly %q (nothing persisted)", got, "[]")
		}
	})

	t.Run("NonPositiveAmount", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		for _, amt := range []string{"0", "-50"} {
			body := `{"kind":"expense","amount":"` + amt + `","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + a + `"}`
			rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("amount %q: status = %d, want 400 (body: %s)", amt, rec.Code, rec.Body.String())
			}
			assertErrorEnvelope(t, rec)
		}
	})

	t.Run("BlankDescription", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		body := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"   ","source_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("InvalidKindShape", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		b := createAccount(t, e, "test-user", "B", "bank", "USD")

		// (a) expense with no account fields.
		body := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"x"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expense no accounts: status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		// (b) transfer with source == destination.
		body = `{"kind":"transfer","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + a + `","destination_account_id":"` + a + `"}`
		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("transfer same account: status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		// (c) transfer from INR to USD account.
		body = `{"kind":"transfer","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + a + `","destination_account_id":"` + b + `"}`
		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("currency mismatch: status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("InvalidOccurredOn", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		for _, d := range []string{"", "garbage"} {
			body := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"` + d + `","description":"x","source_account_id":"` + a + `"}`
			rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("occurred_on %q: status = %d, want 400 (body: %s)", d, rec.Code, rec.Body.String())
			}
			assertErrorEnvelope(t, rec)
		}
	})

	t.Run("AmountRoundTripsExactly", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		body := `{"kind":"expense","amount":"19999.99","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal movement: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(mv, "id")
		if id == "" {
			t.Fatal("id is empty")
		}

		rec = do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal get movement: %v (body: %s)", err, rec.Body.String())
		}
		if v := strVal(got, "amount"); v != "19999.99" {
			t.Errorf("amount = %q, want %q", v, "19999.99")
		}
		if !strings.Contains(rec.Body.String(), `"amount":"19999.99"`) {
			t.Errorf("raw body missing %q (amount must be a JSON string): %s", `"amount":"19999.99"`, rec.Body.String())
		}
		if v := strVal(got, "currency"); v != "INR" {
			t.Errorf("currency = %q, want %q", v, "INR")
		}
	})
}

// TestListMovements exercises GET /api/finance/movements.
func TestListMovements(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements", "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("FilterByAccount", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		b := createAccount(t, e, "test-user", "B", "cash", "INR")

		// Expense on A, expense on B, transfer A -> B.
		mA := createExpense(t, e, a, "a1")
		mB := createExpense(t, e, b, "b1")
		mT := createTransfer(t, e, a, b, "move")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+a, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var list []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if len(list) != 2 {
			t.Fatalf("A filter count = %d, want 2 (body: %s)", len(list), rec.Body.String())
		}
		assertIDSet(t, list, map[string]bool{mA: true, mT: true}, "A filter")

		rec = do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+b, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		list = nil
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if len(list) != 2 {
			t.Fatalf("B filter count = %d, want 2 (body: %s)", len(list), rec.Body.String())
		}
		assertIDSet(t, list, map[string]bool{mB: true, mT: true}, "B filter")
	})

	t.Run("FilterByOccurredRange", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		early := insertMovement(t, e, "test-user", "expense", "10", "INR", "2026-08-01", "early", a, "")
		late := insertMovement(t, e, "test-user", "expense", "20", "INR", "2026-08-20", "late", a, "")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements?from=2026-08-10&to=2026-08-31", "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var list []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if len(list) != 1 {
			t.Fatalf("range count = %d, want 1 (body: %s)", len(list), rec.Body.String())
		}
		if got := strVal(list[0], "id"); got != late {
			t.Errorf("listed id = %q, want the %q movement %q", got, "late", late)
		}
		if got := strVal(list[0], "description"); got != "late" {
			t.Errorf("listed description = %q, want %q", got, "late")
		}
		_ = early
	})

	t.Run("InvalidRange", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements?from=garbage", "test-user", nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// assertIDSet checks that the listed entries' ids are exactly the given set.
func assertIDSet(t *testing.T, list []map[string]any, want map[string]bool, label string) {
	t.Helper()
	got := make(map[string]bool)
	for _, m := range list {
		got[strVal(m, "id")] = true
	}
	for id := range want {
		if !got[id] {
			t.Errorf("%s: missing expected id %q in %v", label, id, got)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("%s: unexpected id %q in %v", label, id, got)
		}
	}
}

// createExpense creates a manual expense movement on source account via API.
func createExpense(t *testing.T, e *testEnv, accountID, desc string) string {
	t.Helper()
	body := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"` + desc + `","source_account_id":"` + accountID + `"}`
	rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expense status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var mv map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
	}
	return strVal(mv, "id")
}

// createTransfer creates a manual transfer movement via API.
func createTransfer(t *testing.T, e *testEnv, srcID, dstID, desc string) string {
	t.Helper()
	body := `{"kind":"transfer","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"` + desc + `","source_account_id":"` + srcID + `","destination_account_id":"` + dstID + `"}`
	rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create transfer status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var mv map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
	}
	return strVal(mv, "id")
}

// TestGetMovement exercises GET /api/finance/movements/{id}.
func TestGetMovement(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpense(t, e, a, "Reliance Digital")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(mv, "kind"); got != "expense" {
			t.Errorf("kind = %q, want %q", got, "expense")
		}
		if got := strVal(mv, "amount"); got != "100" {
			t.Errorf("amount = %q, want %q", got, "100")
		}
		if got := strVal(mv, "currency"); got != "INR" {
			t.Errorf("currency = %q, want %q", got, "INR")
		}
		if got := strVal(mv, "occurred_on"); got != "2026-08-20" {
			t.Errorf("occurred_on = %q, want %q", got, "2026-08-20")
		}
		if got := strVal(mv, "description"); got != "Reliance Digital" {
			t.Errorf("description = %q, want %q", got, "Reliance Digital")
		}
		if got := strVal(mv, "origin"); got != "manual" {
			t.Errorf("origin = %q, want %q", got, "manual")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+unknownUUID, "test-user", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("CrossUser", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpense(t, e, a, "x")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestPatchMovement exercises PATCH /api/finance/movements/{id}.
func TestPatchMovement(t *testing.T) {
	t.Parallel()

	t.Run("Description", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")

		body := `{"kind":"expense","amount":"1250.50","currency":"INR","occurred_on":"2026-08-20","description":"REL DIG 123","source_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var orig map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &orig); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(orig, "id")

		rec = do(t, e.handler, http.MethodPatch, "/api/finance/movements/"+id, "test-user", bytes.NewBufferString(`{"description":"Reliance Digital"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("patch status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var patched map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(patched, "description"); got != "Reliance Digital" {
			t.Errorf("description = %q, want %q", got, "Reliance Digital")
		}
		if got := strVal(patched, "amount"); got != strVal(orig, "amount") {
			t.Errorf("amount = %q, want %q (unchanged)", got, strVal(orig, "amount"))
		}
		if got := strVal(patched, "kind"); got != strVal(orig, "kind") {
			t.Errorf("kind = %q, want %q (unchanged)", got, strVal(orig, "kind"))
		}
		if got := strVal(patched, "source_account_id"); got != strVal(orig, "source_account_id") {
			t.Errorf("source_account_id = %q, want %q (unchanged)", got, strVal(orig, "source_account_id"))
		}
	})

	t.Run("BlankDescription", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpense(t, e, a, "original")

		rec := do(t, e.handler, http.MethodPatch, "/api/finance/movements/"+id, "test-user", bytes.NewBufferString(`{"description":"   "}`), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if got := strVal(mv, "description"); got != "original" {
			t.Errorf("description = %q, want %q (unchanged)", got, "original")
		}
	})

	t.Run("CoreFieldRejected", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		b := createAccount(t, e, "test-user", "B", "cash", "INR")

		body := `{"kind":"expense","amount":"1250.50","currency":"INR","occurred_on":"2026-08-20","description":"orig","source_account_id":"` + a + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var orig map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &orig); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		id := strVal(orig, "id")

		cases := []struct {
			name string
			body string
		}{
			{name: "amount", body: `{"amount":"999"}`},
			{name: "currency", body: `{"currency":"USD"}`},
			{name: "kind", body: `{"kind":"income"}`},
			{name: "occurred_on", body: `{"occurred_on":"2020-01-01"}`},
			{name: "source_account_id", body: `{"source_account_id":"` + b + `"}`},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rec := do(t, e.handler, http.MethodPatch, "/api/finance/movements/"+id, "test-user", bytes.NewBufferString(tc.body), "application/json")
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
				}
				assertErrorEnvelope(t, rec)
			})
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if got := strVal(mv, "amount"); got != strVal(orig, "amount") {
			t.Errorf("amount = %q, want %q (unchanged)", got, strVal(orig, "amount"))
		}
		if got := strVal(mv, "currency"); got != strVal(orig, "currency") {
			t.Errorf("currency = %q, want %q (unchanged)", got, strVal(orig, "currency"))
		}
		if got := strVal(mv, "kind"); got != strVal(orig, "kind") {
			t.Errorf("kind = %q, want %q (unchanged)", got, strVal(orig, "kind"))
		}
		if got := strVal(mv, "occurred_on"); got != strVal(orig, "occurred_on") {
			t.Errorf("occurred_on = %q, want %q (unchanged)", got, strVal(orig, "occurred_on"))
		}
		if got := strVal(mv, "source_account_id"); got != strVal(orig, "source_account_id") {
			t.Errorf("source_account_id = %q, want %q (unchanged)", got, strVal(orig, "source_account_id"))
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodPatch, "/api/finance/movements/"+unknownUUID, "test-user", bytes.NewBufferString(`{"description":"x"}`), "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestDeleteMovement exercises DELETE /api/finance/movements/{id}.
func TestDeleteMovement(t *testing.T) {
	t.Parallel()

	t.Run("Manual", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpense(t, e, a, "only movement")

		rec := do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+id, "test-user", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "" {
			t.Errorf("body = %q, want empty for 204", rec.Body.String())
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusNotFound {
			t.Fatalf("get after delete status = %d, want 404 (body: %s)", get.Code, get.Body.String())
		}

		if bal := getBalance(t, e, "test-user", a); bal != "0" {
			t.Errorf("balance = %q, want %q", bal, "0")
		}
	})

	t.Run("Imported", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := insertImportMovement(t, e, "test-user", "500", "INR", "2026-08-01", "imported expense", a)

		rec := do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+id, "test-user", nil, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if got := strVal(mv, "amount"); got != "500" {
			t.Errorf("amount = %q, want %q (unchanged)", got, "500")
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+unknownUUID, "test-user", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// insertImportMovement inserts one import-origin movement directly via SQL
// and returns its id. import_batch_id stays NULL.
func insertImportMovement(t *testing.T, e *testEnv, OwnerID, amount, currency, occurredOn, desc, source string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var src any
	if source != "" {
		src = source
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
		`INSERT INTO money_movements (owner_id, kind, amount, currency, occurred_on, description, norm_description, origin, source_account_id)
		 VALUES ($1, 'expense', $2::numeric, $3, $4::date, $5, $6, 'import', $7) RETURNING id`,
		OwnerID, amount, currency, occurredOn, desc, strings.ToLower(desc), src,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert import movement: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

// uploadInvoice posts a PDF invoice, asserts 201, fetches the single document
// on the asset, and returns the asset id and document id.
func uploadInvoice(t *testing.T, e *testEnv, userID, filename string) (assetID, docID string) {
	t.Helper()
	rec, asset := e.uploadFile(t, userID, filename, "application/pdf", pdfBytes(16))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	assetID = strVal(asset, "id")
	if assetID == "" {
		t.Fatal("asset id is empty")
	}

	drec := do(t, e.handler, http.MethodGet, "/api/users/"+userID+"/assets/"+assetID+"/documents", "", nil, "")
	if drec.Code != http.StatusOK {
		t.Fatalf("list documents status = %d, want 200 (body: %s)", drec.Code, drec.Body.String())
	}
	var docs []map[string]any
	if err := json.Unmarshal(drec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("unmarshal documents: %v (body: %s)", err, drec.Body.String())
	}
	if len(docs) != 1 {
		t.Fatalf("document count = %d, want 1 (body: %s)", len(docs), drec.Body.String())
	}
	docID = strVal(docs[0], "id")
	if docID == "" {
		t.Fatalf("document id is empty")
	}
	return assetID, docID
}

// TestLinkMovement exercises POST /api/finance/movements/{id}/link.
func TestLinkMovement(t *testing.T) {
	t.Parallel()

	t.Run("Created", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "invoice purchase")

		_, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if got := strVal(mv, "linked_document_id"); got != docID {
			t.Errorf("linked_document_id = %q, want %q", got, docID)
		}
		if got := strVal(mv, "link_creator"); got != "manual" {
			t.Errorf("link_creator = %q, want %q", got, "manual")
		}
		if v, ok := mv["link_conflicting"]; !ok || v != false {
			t.Errorf("link_conflicting = %v, want false", v)
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if v := strVal(got, "linked_document_id"); v != docID {
			t.Errorf("get linked_document_id = %q, want %q", v, docID)
		}
		if v := strVal(got, "link_creator"); v != "manual" {
			t.Errorf("get link_creator = %q, want %q", v, "manual")
		}
	})

	t.Run("IdempotentSameLink", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "invoice purchase")
		_, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		body := `{"document_id":"` + docID + `"}`
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("first link status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		// Second identical link must be a no-op success.
		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("second link status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if v := strVal(mv, "linked_document_id"); v != docID {
			t.Errorf("linked_document_id = %q, want %q (still linked)", v, docID)
		}
		if v := strVal(mv, "amount"); v != "100" {
			t.Errorf("amount = %q, want %q (unchanged)", v, "100")
		}
	})

	t.Run("MovementAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "invoice purchase")

		_, d1 := uploadInvoice(t, e, "test-user", "invoice-1.pdf")
		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+d1+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("first link status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		// Second invoice with a different serial so a new document is created.
		e.setLLMPayload(`{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-002","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`)
		_, d2 := uploadInvoice(t, e, "test-user", "invoice-2.pdf")
		if d2 == d1 {
			t.Fatalf("second upload produced the same document id %q, want a new document", d2)
		}

		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+d2+`"}`), "application/json")
		if rec.Code != http.StatusConflict {
			t.Fatalf("conflict status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if v := strVal(mv, "linked_document_id"); v != d1 {
			t.Errorf("linked_document_id = %q, want %q (unchanged)", v, d1)
		}
	})

	t.Run("DocumentAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		m1 := createExpenseAmount(t, e, a, "100", "one")
		m2 := createExpenseAmount(t, e, a, "200", "two")

		_, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+m1+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("m1 link status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements/"+m2+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusConflict {
			t.Fatalf("m2 link status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+m1, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get m1 status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if v := strVal(mv, "linked_document_id"); v != docID {
			t.Errorf("m1 linked_document_id = %q, want %q (unchanged)", v, docID)
		}
	})

	t.Run("CrossUserDocument", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "invoice purchase")

		// User B uploads its own invoice (same serial, distinct userID asset).
		_, docB := uploadInvoice(t, e, "test-user-b", "invoice-b.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docB+`"}`), "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if _, ok := mv["linked_document_id"]; ok {
			t.Errorf("linked_document_id present, want absent: %s", get.Body.String())
		}
	})

	t.Run("UnknownDocument", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "x")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+unknownUUID+`"}`), "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingDocumentID", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "x")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{}`), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// createExpenseAmount creates a manual expense with the given amount on account.
func createExpenseAmount(t *testing.T, e *testEnv, accountID, amount, desc string) string {
	t.Helper()
	body := `{"kind":"expense","amount":"` + amount + `","currency":"INR","occurred_on":"2026-08-20","description":"` + desc + `","source_account_id":"` + accountID + `"}`
	rec := do(t, e.handler, http.MethodPost, "/api/finance/movements", "test-user", bytes.NewBufferString(body), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expense status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var mv map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
	}
	return strVal(mv, "id")
}

// TestUnlinkMovement exercises DELETE /api/finance/movements/{id}/link.
func TestUnlinkMovement(t *testing.T) {
	t.Parallel()

	t.Run("UnlinkRemovesLink", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "invoice purchase")
		_, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("link status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		rec = do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+id+"/link", "test-user", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("unlink status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if _, ok := mv["linked_document_id"]; ok {
			t.Errorf("linked_document_id present after unlink, want absent: %s", get.Body.String())
		}

		// Re-link must succeed: the document is linkable again.
		rec = do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("relink status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("UnlinkedNoOp", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "100", "never linked")

		rec := do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+id+"/link", "test-user", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+unknownUUID+"/link", "test-user", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestLinkConflict exercises the link_conflicting flag on POST link.
func TestLinkConflict(t *testing.T) {
	t.Parallel()

	t.Run("DisagreeingValuesRetainedAndFlagged", func(t *testing.T) {
		t.Parallel()
		// Default happyPayload: price "39999.99" INR.
		e := newEnv(t, envOpts{})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "40000", "invoice purchase")

		assetID, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if v, ok := mv["link_conflicting"]; !ok || v != true {
			t.Errorf("link_conflicting = %v, want true", v)
		}

		get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
		if get.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, get.Body.String())
		}
		if v := strVal(got, "amount"); v != "40000" {
			t.Errorf("amount = %q, want %q (unchanged)", v, "40000")
		}

		arec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+assetID, "", nil, "")
		if arec.Code != http.StatusOK {
			t.Fatalf("get asset status = %d, want 200 (body: %s)", arec.Code, arec.Body.String())
		}
		if !strings.Contains(arec.Body.String(), `"price":"39999.99"`) {
			t.Errorf("asset price changed (want %q): %s", `"price":"39999.99"`, arec.Body.String())
		}
	})

	t.Run("AgreeingValuesNoConflict", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"40000","currency":"INR","metadata":{}}`,
		})
		a := createAccount(t, e, "test-user", "A", "bank", "INR")
		id := createExpenseAmount(t, e, a, "40000", "invoice purchase")
		_, docID := uploadInvoice(t, e, "test-user", "invoice.pdf")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/movements/"+id+"/link", "test-user", bytes.NewBufferString(`{"document_id":"`+docID+`"}`), "application/json")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var mv map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &mv); err != nil {
			t.Fatalf("unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if v, ok := mv["link_conflicting"]; !ok || v != false {
			t.Errorf("link_conflicting = %v, want false", v)
		}
	})
}

// TestMovementsMissingUser exercises the user-identity contract: every
// movement endpoint with an empty {userId} path segment is rejected with 400.
func TestMovementsMissingUser(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	validBody := `{"kind":"expense","amount":"100","currency":"INR","occurred_on":"2026-08-20","description":"x","source_account_id":"` + unknownUUID + `"}`

	cases := []struct {
		name   string
		method string
		path   string
		body   *bytes.Buffer
		cType  string
	}{
		{name: "POST /api/users//finance/movements", method: http.MethodPost, path: "/api/users//finance/movements", body: bytes.NewBufferString(validBody), cType: "application/json"},
		{name: "GET /api/users//finance/movements", method: http.MethodGet, path: "/api/users//finance/movements"},
		{name: "GET /api/users//finance/movements/{id}", method: http.MethodGet, path: "/api/users//finance/movements/" + unknownUUID},
		{name: "PATCH /api/users//finance/movements/{id}", method: http.MethodPatch, path: "/api/users//finance/movements/" + unknownUUID, body: bytes.NewBufferString(`{"description":"x"}`), cType: "application/json"},
		{name: "DELETE /api/users//finance/movements/{id}", method: http.MethodDelete, path: "/api/users//finance/movements/" + unknownUUID},
		{name: "POST /api/users//finance/movements/{id}/link", method: http.MethodPost, path: "/api/users//finance/movements/" + unknownUUID + "/link", body: bytes.NewBufferString(`{"document_id":"` + unknownUUID + `"}`), cType: "application/json"},
		{name: "DELETE /api/users//finance/movements/{id}/link", method: http.MethodDelete, path: "/api/users//finance/movements/" + unknownUUID + "/link"},
	}
	// The user middleware rejects an empty {userId} path segment with 400.
	for _, tc := range cases {
		tc := tc
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

// TestMovementUserScoping verifies movements are fully userID-scoped.
func TestMovementUserScoping(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	a := createAccount(t, e, "test-user", "A", "bank", "INR")
	id := createExpense(t, e, a, "userID A expense")

	// User B sees no movements and cannot read/patch/delete userID A's.
	rec := do(t, e.handler, http.MethodGet, "/api/finance/movements", "test-user-b", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("list body = %q, want exactly %q (never null)", got, "[]")
	}

	rec = do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user-b", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	assertErrorEnvelope(t, rec)

	rec = do(t, e.handler, http.MethodPatch, "/api/finance/movements/"+id, "test-user-b", bytes.NewBufferString(`{"description":"hacked"}`), "application/json")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	assertErrorEnvelope(t, rec)

	rec = do(t, e.handler, http.MethodDelete, "/api/finance/movements/"+id, "test-user-b", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	assertErrorEnvelope(t, rec)

	// User A still sees its movement.
	get := do(t, e.handler, http.MethodGet, "/api/finance/movements/"+id, "test-user", nil, "")
	if get.Code != http.StatusOK {
		t.Fatalf("userID A get status = %d, want 200 (body: %s)", get.Code, get.Body.String())
	}
}
