// Package api integration tests for the statement import API surface:
//
//	POST /api/finance/import-batches               multipart upload, field "file" + field "account_id"
//	GET  /api/finance/import-batches               list preview/committed/discard batches for the tenant
//	GET  /api/finance/import-batches/{id}          single batch with parsed lines
//	POST /api/finance/import-batches/{id}/commit   create movements for valid lines
//	POST /api/finance/import-batches/{id}/discard  keep the source, create no movements
//
// These tests are the RED phase for task 9.5: the routes are not registered
// yet, so the suite must compile and fail at runtime.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// uploadStatement posts a multipart statement upload (file part "file" + field "account_id")
// to POST /api/finance/import-batches and returns the recorded response.
func uploadStatement(t *testing.T, e *testEnv, tenant, filename, contentType string, content []byte, accountID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := w.WriteField("account_id", accountID); err != nil {
		t.Fatalf("write account_id field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return do(t, e.handler, http.MethodPost, "/api/finance/import-batches", tenant, &buf, w.FormDataContentType())
}

// createTestAccount creates a bank account named "Import Test" and returns its id.
func createTestAccount(t *testing.T, e *testEnv, tenant string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"name":"Import Test","type":"bank","currency":"INR"}`)
	rec := do(t, e.handler, http.MethodPost, "/api/finance/accounts", tenant, body, "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
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

// insertMovementWithRef inserts one manual movement directly via SQL with an
// external_reference set, and returns its id.
func insertMovementWithRef(t *testing.T, e *testEnv, tenantID, amount, currency, occurredOn, desc, account, ref string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var id string
	err := e.pool.QueryRow(ctx,
		`INSERT INTO money_movements (tenant_id, kind, amount, currency, occurred_on, description, norm_description, origin, source_account_id, external_reference)
		 VALUES ($1, 'expense', $2::numeric, $3, $4::date, $5, $6, 'manual', $7, $8) RETURNING id`,
		tenantID, amount, currency, occurredOn, desc, strings.ToLower(desc), account, ref,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert movement with ref %s: %v", ref, err)
	}
	return id
}

// readTestdata loads a fixture file from api/testdata.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return b
}

// batchJSON is the import batch response envelope.
func batchJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal batch JSON: %v (body: %s)", err, rec.Body.String())
	}
	return m
}

// lineWithRef finds the batch line with the given line_ref.
func lineWithRef(t *testing.T, batch map[string]any, ref int) map[string]any {
	t.Helper()
	lines, ok := batch["lines"].([]any)
	if !ok {
		t.Fatalf("lines is not a JSON array: %v", batch["lines"])
	}
	for _, raw := range lines {
		line, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("line entry is not an object: %v", raw)
		}
		if n, ok := line["line_ref"].(float64); ok && int(n) == ref {
			return line
		}
	}
	t.Fatalf("no line with line_ref %d: %v", ref, lines)
	return nil
}

// intVal returns the numeric value of key k in m as an int (0 when absent).
func intVal(m map[string]any, k string) int {
	if n, ok := m[k].(float64); ok {
		return int(n)
	}
	return 0
}

// requireBatchUpload uploads content and requires a 201 with a batch id.
func requireBatchUpload(t *testing.T, e *testEnv, tenant, filename, contentType string, content []byte, accountID string) map[string]any {
	t.Helper()
	rec := uploadStatement(t, e, tenant, filename, contentType, content, accountID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	batch := batchJSON(t, rec)
	if id := strVal(batch, "id"); id == "" {
		t.Fatalf("batch id is empty: %s", rec.Body.String())
	}
	return batch
}

// sourceCount returns the number of sources rows with the given filename for
// the tenant. sources has FORCE RLS (multi-tenant-isolation), so the counting
// transaction must bind app.tenant_id or every row is filtered out.
func sourceCount(t *testing.T, e *testEnv, tenant, filename string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sources WHERE filename = $1`, filename).Scan(&n); err != nil {
		t.Fatalf("count sources: %v", err)
	}
	return n
}

// TestCreateImportBatch exercises POST /api/finance/import-batches.
func TestCreateImportBatch(t *testing.T) {
	t.Parallel()

	t.Run("HappyPathCSV", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		batch := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)

		if got := strVal(batch, "state"); got != "preview" {
			t.Errorf("state = %q, want %q", got, "preview")
		}
		if got := strVal(batch, "format"); got != "csv" {
			t.Errorf("format = %q, want %q", got, "csv")
		}
		if got := strVal(batch, "filename"); got != "statement_ok.csv" {
			t.Errorf("filename = %q, want %q", got, "statement_ok.csv")
		}
		src, ok := batch["source"].(map[string]any)
		if !ok {
			t.Fatalf("source is not a JSON object: %v", batch["source"])
		}
		if got := strVal(src, "id"); got == "" {
			t.Error("source.id is empty, want non-empty")
		}
		if got := strVal(src, "filename"); got != "statement_ok.csv" {
			t.Errorf("source.filename = %q, want %q", got, "statement_ok.csv")
		}
		if got := intVal(batch, "line_count_valid"); got != 3 {
			t.Errorf("line_count_valid = %d, want 3", got)
		}
		if got := intVal(batch, "line_count_error"); got != 1 {
			t.Errorf("line_count_error = %d, want 1", got)
		}
		lines, ok := batch["lines"].([]any)
		if !ok || len(lines) != 4 {
			t.Fatalf("lines length = %d, want 4", len(lines))
		}

		l2 := lineWithRef(t, batch, 2)
		if got := strVal(l2, "status"); got != "valid" {
			t.Errorf("line 2 status = %q, want %q", got, "valid")
		}
		if got := strVal(l2, "amount"); got != "250.50" {
			t.Errorf("line 2 amount = %q, want %q", got, "250.50")
		}
		if got := strVal(l2, "direction"); got != "out" {
			t.Errorf("line 2 direction = %q, want %q", got, "out")
		}
		if got := strVal(l2, "external_reference"); got != "TXN-101" {
			t.Errorf("line 2 external_reference = %q, want %q", got, "TXN-101")
		}
		if got := strVal(l2, "occurred_on"); got != "2026-08-21" {
			t.Errorf("line 2 occurred_on = %q, want %q", got, "2026-08-21")
		}

		l4 := lineWithRef(t, batch, 4)
		if got := strVal(l4, "status"); got != "error" {
			t.Errorf("line 4 status = %q, want %q", got, "error")
		}
		if got := strVal(l4, "error_reason"); got == "" {
			t.Error("line 4 error_reason is empty, want non-empty")
		}

		// Preview creates no movements.
		rec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("movements status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Errorf("movements body = %q, want exactly %q (preview creates none)", got, "[]")
		}
		if bal := getBalance(t, e, "test-tenant", acct); bal != "0" {
			t.Errorf("balance = %q, want %q (preview creates no movements)", bal, "0")
		}
	})

	t.Run("HappyPathPDF", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		batch := requireBatchUpload(t, e, "test-tenant", "statement_text.pdf", "application/pdf", readTestdata(t, "statement_text.pdf"), acct)

		if got := strVal(batch, "state"); got != "preview" {
			t.Errorf("state = %q, want %q", got, "preview")
		}
		if got := strVal(batch, "format"); got != "pdf" {
			t.Errorf("format = %q, want %q", got, "pdf")
		}
		if got := intVal(batch, "line_count_valid"); got != 1 {
			t.Errorf("line_count_valid = %d, want 1", got)
		}
		lines, ok := batch["lines"].([]any)
		if !ok || len(lines) != 1 {
			t.Fatalf("lines length = %d, want 1", len(lines))
		}
		l, ok := lines[0].(map[string]any)
		if !ok {
			t.Fatalf("line 0 is not an object: %v", lines[0])
		}
		if got := strVal(l, "status"); got != "valid" {
			t.Errorf("line 0 status = %q, want %q", got, "valid")
		}
		if got := strVal(l, "amount"); got != "400.00" {
			t.Errorf("line 0 amount = %q, want %q", got, "400.00")
		}
		if got := strVal(l, "direction"); got != "out" {
			t.Errorf("line 0 direction = %q, want %q", got, "out")
		}
		if got := strVal(l, "description"); got != "Text Layer Buy" {
			t.Errorf("line 0 description = %q, want %q", got, "Text Layer Buy")
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte("\x00"), 32)...)
		rec := uploadStatement(t, e, "test-tenant", "fake.png", "image/png", png, acct)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		list := do(t, e.handler, http.MethodGet, "/api/finance/import-batches", "test-tenant", nil, "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200 (body: %s)", list.Code, list.Body.String())
		}
		if got := strings.TrimSpace(list.Body.String()); got != "[]" {
			t.Errorf("list body = %q, want exactly %q (no batch persisted)", got, "[]")
		}
	})

	t.Run("Oversize", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{maxBytes: 64})
		acct := createTestAccount(t, e, "test-tenant")

		content := []byte(strings.Repeat("2026-08-20,-1.00,Pad\n", 10))
		rec := uploadStatement(t, e, "test-tenant", "pad.csv", "text/csv", content, acct)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("UnknownAccount", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		rec := uploadStatement(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), unknownUUID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingFileField", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.WriteField("account_id", acct); err != nil {
			t.Fatalf("write account_id field: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}
		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches", "test-tenant", &buf, w.FormDataContentType())
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingAccountID", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		body, ct := buildMultipart(t, "file", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"))
		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches", "test-tenant", body, ct)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("BlankAccountID", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="file"; filename="statement_ok.csv"`},
			"Content-Type":        {"text/csv"},
		})
		if err != nil {
			t.Fatalf("create multipart part: %v", err)
		}
		if _, err := part.Write(readTestdata(t, "statement_ok.csv")); err != nil {
			t.Fatalf("write multipart part: %v", err)
		}
		if err := w.WriteField("account_id", "   "); err != nil {
			t.Fatalf("write account_id field: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close multipart writer: %v", err)
		}
		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches", "test-tenant", &buf, w.FormDataContentType())
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("ImageOnlyPDF", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		rec := uploadStatement(t, e, "test-tenant", "statement_image_only.pdf", "application/pdf", readTestdata(t, "statement_image_only.pdf"), acct)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		// The source is retained despite the rejection.
		if n := sourceCount(t, e, "test-tenant", "statement_image_only.pdf"); n != 1 {
			t.Errorf("sources count for statement_image_only.pdf = %d, want 1 (source retained)", n)
		}
	})

	t.Run("LineLimitExceededSmall", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{maxStatementLines: 5})
		acct := createTestAccount(t, e, "test-tenant")

		var b strings.Builder
		for i := 1; i <= 6; i++ {
			fmt.Fprintf(&b, "2026-08-20,-1.00,Line %d\n", i)
		}
		rec := uploadStatement(t, e, "test-tenant", "small_limit.csv", "text/csv", []byte(b.String()), acct)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		if n := sourceCount(t, e, "test-tenant", "small_limit.csv"); n != 1 {
			t.Errorf("sources count for small_limit.csv = %d, want 1 (source retained)", n)
		}
	})

	t.Run("LineLimitExceededDefault", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		var b strings.Builder
		for i := 1; i <= 100001; i++ {
			fmt.Fprintf(&b, "2026-08-20,-1.00,Row %d\n", i)
		}
		rec := uploadStatement(t, e, "test-tenant", "big_statement.csv", "text/csv", []byte(b.String()), acct)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		list := do(t, e.handler, http.MethodGet, "/api/finance/import-batches", "test-tenant", nil, "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200 (body: %s)", list.Code, list.Body.String())
		}
		if got := strings.TrimSpace(list.Body.String()); got != "[]" {
			t.Errorf("list body = %q, want exactly %q (no batch persisted)", got, "[]")
		}
	})
}

// TestGetImportBatch exercises GET /api/finance/import-batches/{id}.
func TestGetImportBatch(t *testing.T) {
	t.Parallel()

	t.Run("PreviewRereadable", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		batch := batchJSON(t, rec)

		if got := strVal(batch, "state"); got != "preview" {
			t.Errorf("state = %q, want %q", got, "preview")
		}
		if got := intVal(batch, "line_count_valid"); got != 3 {
			t.Errorf("line_count_valid = %d, want 3", got)
		}
		lines, ok := batch["lines"].([]any)
		if !ok || len(lines) != 4 {
			t.Fatalf("lines length = %d, want 4", len(lines))
		}
		for i, raw := range lines {
			line, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("line %d is not an object: %v", i, raw)
			}
			if got := intVal(line, "line_ref"); got != i+1 {
				t.Errorf("line %d line_ref = %d, want %d", i, got, i+1)
			}
		}
		src, ok := batch["source"].(map[string]any)
		if !ok {
			t.Fatalf("source is not a JSON object: %v", batch["source"])
		}
		if got := strVal(src, "id"); got == "" {
			t.Error("source.id is empty, want non-empty")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+unknownUUID, "test-tenant", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("CrossTenant", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListImportBatches exercises GET /api/finance/import-batches.
func TestListImportBatches(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Errorf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("WithBatches", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		requireBatchUpload(t, e, "test-tenant", "statement_mixed.csv", "text/csv", readTestdata(t, "statement_mixed.csv"), acct)

		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var batches []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &batches); err != nil {
			t.Fatalf("unmarshal batches: %v (body: %s)", err, rec.Body.String())
		}
		if len(batches) != 2 {
			t.Fatalf("batch count = %d, want 2 (body: %s)", len(batches), rec.Body.String())
		}
		for i, b := range batches {
			if got := strVal(b, "id"); got == "" {
				t.Errorf("entry %d id is empty, want non-empty", i)
			}
			if got := strVal(b, "state"); got != "preview" {
				t.Errorf("entry %d state = %q, want %q", i, got, "preview")
			}
			src, ok := b["source"].(map[string]any)
			if !ok {
				t.Fatalf("entry %d source is not a JSON object: %v", i, b["source"])
			}
			if got := strVal(src, "id"); got == "" {
				t.Errorf("entry %d source.id is empty, want non-empty", i)
			}
		}

		okEntry, mixedEntry, foundOk, foundMixed := map[string]any{}, map[string]any{}, false, false
		for _, b := range batches {
			switch strVal(b, "filename") {
			case "statement_ok.csv":
				okEntry, foundOk = b, true
			case "statement_mixed.csv":
				mixedEntry, foundMixed = b, true
			}
		}
		if !foundOk {
			t.Fatalf("no entry with filename statement_ok.csv: %s", rec.Body.String())
		}
		if !foundMixed {
			t.Fatalf("no entry with filename statement_mixed.csv: %s", rec.Body.String())
		}
		if got := intVal(okEntry, "line_count_valid"); got != 3 {
			t.Errorf("ok line_count_valid = %d, want 3", got)
		}
		if got := intVal(okEntry, "line_count_error"); got != 1 {
			t.Errorf("ok line_count_error = %d, want 1", got)
		}
		if got := intVal(mixedEntry, "line_count_valid"); got != 2 {
			t.Errorf("mixed line_count_valid = %d, want 2", got)
		}
		if got := intVal(mixedEntry, "line_count_duplicate"); got != 1 {
			t.Errorf("mixed line_count_duplicate = %d, want 1", got)
		}
	})
}

// TestCommitImportBatch exercises POST /api/finance/import-batches/{id}/commit.
func TestCommitImportBatch(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("commit status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal commit result: %v (body: %s)", err, rec.Body.String())
		}
		if got := intVal(result, "created"); got != 3 {
			t.Errorf("created = %d, want 3", got)
		}
		if got := intVal(result, "skipped"); got != 1 {
			t.Errorf("skipped = %d, want 1", got)
		}

		movRec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
		if movRec.Code != http.StatusOK {
			t.Fatalf("movements status = %d, want 200 (body: %s)", movRec.Code, movRec.Body.String())
		}
		var movs []map[string]any
		if err := json.Unmarshal(movRec.Body.Bytes(), &movs); err != nil {
			t.Fatalf("unmarshal movements: %v (body: %s)", err, movRec.Body.String())
		}
		if len(movs) != 3 {
			t.Fatalf("movement count = %d, want 3 (body: %s)", len(movs), movRec.Body.String())
		}
		seenLines := map[int]bool{}
		for i, m := range movs {
			if got := strVal(m, "origin"); got != "import" {
				t.Errorf("movement %d origin = %q, want %q", i, got, "import")
			}
			if got := strVal(m, "import_batch_id"); got != id {
				t.Errorf("movement %d import_batch_id = %q, want %q", i, got, id)
			}
			n := intVal(m, "import_line")
			if n < 1 || n > 3 {
				t.Errorf("movement %d import_line = %d, want in {1,2,3}", i, n)
			}
			if seenLines[n] {
				t.Errorf("import_line %d appears more than once", n)
			}
			seenLines[n] = true
			if n == 2 {
				if got := strVal(m, "external_reference"); got != "TXN-101" {
					t.Errorf("movement import_line 2 external_reference = %q, want %q", got, "TXN-101")
				}
			}
		}

		if bal := getBalance(t, e, "test-tenant", acct); bal != "129.50" {
			t.Errorf("balance = %q, want %q (500.00 in, 250.50+120.00 out)", bal, "129.50")
		}

		getRec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant", nil, "")
		if getRec.Code != http.StatusOK {
			t.Fatalf("get batch status = %d, want 200 (body: %s)", getRec.Code, getRec.Body.String())
		}
		if got := strVal(batchJSON(t, getRec), "state"); got != "committed" {
			t.Errorf("state = %q, want %q", got, "committed")
		}

		// RecommitIdempotent: a second commit is a no-op with the same counts.
		rec = do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("recommit status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal recommit result: %v (body: %s)", err, rec.Body.String())
		}
		if got := intVal(result, "created"); got != 3 {
			t.Errorf("recommit created = %d, want 3", got)
		}
		if got := intVal(result, "skipped"); got != 1 {
			t.Errorf("recommit skipped = %d, want 1", got)
		}

		movRec = do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
		if movRec.Code != http.StatusOK {
			t.Fatalf("movements after recommit status = %d, want 200 (body: %s)", movRec.Code, movRec.Body.String())
		}
		if err := json.Unmarshal(movRec.Body.Bytes(), &movs); err != nil {
			t.Fatalf("unmarshal movements after recommit: %v (body: %s)", err, movRec.Body.String())
		}
		if len(movs) != 3 {
			t.Errorf("movement count after recommit = %d, want 3 (body: %s)", len(movs), movRec.Body.String())
		}
	})

	t.Run("ZeroValidLines", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")

		content := []byte("2026-08-20,bad-amount,desc\nnotadate,5.00,desc\n")
		created := requireBatchUpload(t, e, "test-tenant", "zero_valid.csv", "text/csv", content, acct)
		if got := intVal(created, "line_count_error"); got != 2 {
			t.Fatalf("line_count_error = %d, want 2", got)
		}
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("commit status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal commit result: %v (body: %s)", err, rec.Body.String())
		}
		if got := intVal(result, "created"); got != 0 {
			t.Errorf("created = %d, want 0", got)
		}
		if got := intVal(result, "skipped"); got != 2 {
			t.Errorf("skipped = %d, want 2", got)
		}

		getRec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant", nil, "")
		if getRec.Code != http.StatusOK {
			t.Fatalf("get batch status = %d, want 200 (body: %s)", getRec.Code, getRec.Body.String())
		}
		if got := strVal(batchJSON(t, getRec), "state"); got != "committed" {
			t.Errorf("state = %q, want %q", got, "committed")
		}

		movRec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
		if movRec.Code != http.StatusOK {
			t.Fatalf("movements status = %d, want 200 (body: %s)", movRec.Code, movRec.Body.String())
		}
		if got := strings.TrimSpace(movRec.Body.String()); got != "[]" {
			t.Errorf("movements body = %q, want exactly %q", got, "[]")
		}
	})

	t.Run("DiscardedConflict", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		disc := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/discard", "test-tenant", nil, "")
		if disc.Code != http.StatusOK {
			t.Fatalf("discard status = %d, want 200 (body: %s)", disc.Code, disc.Body.String())
		}

		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("commit status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		movRec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
		if movRec.Code != http.StatusOK {
			t.Fatalf("movements status = %d, want 200 (body: %s)", movRec.Code, movRec.Body.String())
		}
		if got := strings.TrimSpace(movRec.Body.String()); got != "[]" {
			t.Errorf("movements body = %q, want exactly %q", got, "[]")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+unknownUUID+"/commit", "test-tenant", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestDiscardImportBatch exercises POST /api/finance/import-batches/{id}/discard.
func TestDiscardImportBatch(t *testing.T) {
	t.Parallel()

	t.Run("PreviewDiscarded", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/discard", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("discard status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		batch := batchJSON(t, rec)

		if got := strVal(batch, "state"); got != "discarded" {
			t.Errorf("state = %q, want %q", got, "discarded")
		}
		lines, ok := batch["lines"].([]any)
		if !ok || len(lines) != 4 {
			t.Fatalf("lines length = %d, want 4 (still readable)", len(lines))
		}
		src, ok := batch["source"].(map[string]any)
		if !ok {
			t.Fatalf("source is not a JSON object: %v", batch["source"])
		}
		if got := strVal(src, "id"); got == "" {
			t.Error("source.id is empty, want non-empty")
		}

		// ReDiscardIdempotent: a second discard is a no-op.
		rec = do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/discard", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("re-discard status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strVal(batchJSON(t, rec), "state"); got != "discarded" {
			t.Errorf("re-discard state = %q, want %q", got, "discarded")
		}
	})

	t.Run("CommittedConflict", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		commit := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
		if commit.Code != http.StatusOK {
			t.Fatalf("commit status = %d, want 200 (body: %s)", commit.Code, commit.Body.String())
		}

		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/discard", "test-tenant", nil, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("discard status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		getRec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant", nil, "")
		if getRec.Code != http.StatusOK {
			t.Fatalf("get batch status = %d, want 200 (body: %s)", getRec.Code, getRec.Body.String())
		}
		if got := strVal(batchJSON(t, getRec), "state"); got != "committed" {
			t.Errorf("state = %q, want %q (still committed)", got, "committed")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+unknownUUID+"/discard", "test-tenant", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestImportTenantScoping verifies import batches are fully tenant-scoped.
func TestImportTenantScoping(t *testing.T) {
	t.Parallel()

	t.Run("ForeignBatchNotVisible", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})
		acct := createTestAccount(t, e, "test-tenant")
		created := requireBatchUpload(t, e, "test-tenant", "statement_ok.csv", "text/csv", readTestdata(t, "statement_ok.csv"), acct)
		id := strVal(created, "id")

		rec := do(t, e.handler, http.MethodGet, "/api/finance/import-batches/"+id, "test-tenant-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("get status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		rec = do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("commit status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		rec = do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/discard", "test-tenant-b", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("discard status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)

		list := do(t, e.handler, http.MethodGet, "/api/finance/import-batches", "test-tenant-b", nil, "")
		if list.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200 (body: %s)", list.Code, list.Body.String())
		}
		if got := strings.TrimSpace(list.Body.String()); got != "[]" {
			t.Errorf("list body = %q, want exactly %q", got, "[]")
		}
	})

	t.Run("DuplicateDetectionIgnoresOtherTenants", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envOpts{})

		// Tenant B has a movement with external reference TXN-555.
		acctB := createTestAccount(t, e, "test-tenant-b")
		insertMovementWithRef(t, e, "test-tenant-b", "100.00", "INR", "2026-08-24", "other tenant txn", acctB, "TXN-555")

		// Tenant A uploads a line with the same external reference: it must be
		// valid, not duplicate.
		acctA := createTestAccount(t, e, "test-tenant")
		content := []byte("2026-08-25,-777.00,Other Tenant Ref,TXN-555\n")
		created := requireBatchUpload(t, e, "test-tenant", "other_ref.csv", "text/csv", content, acctA)

		lines, ok := created["lines"].([]any)
		if !ok || len(lines) != 1 {
			t.Fatalf("lines length = %d, want 1", len(lines))
		}
		line, ok := lines[0].(map[string]any)
		if !ok {
			t.Fatalf("line 0 is not an object: %v", lines[0])
		}
		if got := strVal(line, "status"); got == "duplicate" {
			t.Errorf("line status = %q, must not be duplicate (other tenant's ref TXN-555 must be ignored)", got)
		}
		if got := strVal(line, "status"); got != "valid" {
			t.Errorf("line status = %q, want %q", got, "valid")
		}
	})
}

// TestAutoLinkObservable verifies that committing an import batch auto-links
// the movement to an existing document whose asset identity matches.
func TestAutoLinkObservable(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{
		llmPayload: `{"classification":"invoice","brand":"Reliance","model":"Digital","serial_number":"AUTOLINK-1","purchase_date":"2026-08-20","warranty_end":"2029-08-20","price":"40000","currency":"INR","metadata":{}}`,
	})

	// Upload a document whose extracted identity (Reliance Digital AUTOLINK-1)
	// matches the imported line's brand+model description.
	up, asset := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(24))
	if up.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
	}
	assetID := strVal(asset, "id")
	if assetID == "" {
		t.Fatalf("asset id is empty")
	}

	docsRec := do(t, e.handler, http.MethodGet, "/api/users/test-tenant/assets/"+assetID+"/documents", "", nil, "")
	if docsRec.Code != http.StatusOK {
		t.Fatalf("documents status = %d, want 200 (body: %s)", docsRec.Code, docsRec.Body.String())
	}
	var docs []map[string]any
	if err := json.Unmarshal(docsRec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("unmarshal documents: %v (body: %s)", err, docsRec.Body.String())
	}
	if len(docs) != 1 {
		t.Fatalf("document count = %d, want 1 (body: %s)", len(docs), docsRec.Body.String())
	}
	docID := strVal(docs[0], "id")
	if docID == "" {
		t.Fatalf("document id is empty")
	}

	acct := createTestAccount(t, e, "test-tenant")
	content := []byte("2026-08-20,-40000,Reliance Digital,TXN-900\n")
	created := requireBatchUpload(t, e, "test-tenant", "autolink.csv", "text/csv", content, acct)
	id := strVal(created, "id")

	rec := do(t, e.handler, http.MethodPost, "/api/finance/import-batches/"+id+"/commit", "test-tenant", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("commit status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal commit result: %v (body: %s)", err, rec.Body.String())
	}
	if got := intVal(result, "created"); got != 1 {
		t.Errorf("created = %d, want 1", got)
	}
	if got := intVal(result, "skipped"); got != 0 {
		t.Errorf("skipped = %d, want 0", got)
	}

	movRec := do(t, e.handler, http.MethodGet, "/api/finance/movements?account_id="+acct, "test-tenant", nil, "")
	if movRec.Code != http.StatusOK {
		t.Fatalf("movements status = %d, want 200 (body: %s)", movRec.Code, movRec.Body.String())
	}
	var movs []map[string]any
	if err := json.Unmarshal(movRec.Body.Bytes(), &movs); err != nil {
		t.Fatalf("unmarshal movements: %v (body: %s)", err, movRec.Body.String())
	}
	if len(movs) != 1 {
		t.Fatalf("movement count = %d, want 1 (body: %s)", len(movs), movRec.Body.String())
	}
	if got := strVal(movs[0], "linked_document_id"); got != docID {
		t.Errorf("linked_document_id = %q, want document id %q (auto-linked)", got, docID)
	}
	if got := strVal(movs[0], "link_creator"); got != "auto" {
		t.Errorf("link_creator = %q, want %q", got, "auto")
	}
}
