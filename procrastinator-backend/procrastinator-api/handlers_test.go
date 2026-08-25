// Package api integration tests for the HTTP API surface:
//
//	api.New(svc, assets, docs, maxBytes) *api.Server
//	(*api.Server).Routes() http.Handler
//
// Endpoints (all require the X-Tenant-ID header):
//
//	POST /api/documents              multipart upload, field "file"
//	GET  /api/assets                 list assets for the tenant
//	GET  /api/assets/{id}            single asset
//	GET  /api/assets/{id}/documents  documents for one asset
//
// Every test runs against a real Postgres database in a private schema and a
// real llm.Client pointed at a per-test httptest fake, so the full stack
// (handler → service → extractor → storage → postgres) is exercised.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-core/ingest"
	"procrastinator-backend/procrastinator-infra/filestorage"
	"procrastinator-backend/procrastinator-infra/llm"
	"procrastinator-backend/procrastinator-infra/postgres"
)

const migrationsDir = "../migrations"

// defaultMaxBytes is the upload size limit used unless a test opts otherwise.
const defaultMaxBytes int64 = 20 * 1024 * 1024

// happyPayload is the extraction payload returned by the fake LLM for the
// happy-path invoice upload.
const happyPayload = `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"WM-2024-001","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"39999.99","currency":"INR","metadata":{"invoice_number":"INVNAG2302754","customer_name":"ACME CORP"}}`

// noIdentityPayload has no serial, brand, or model: identity resolution must
// reject it.
const noIdentityPayload = `{"classification":"other"}`

// TestMain reads PROCRASTINATOR_TEST_DATABASE_URL once — the only env access
// in this package. When unset, the suite skips cleanly (exit 0). When set, it
// verifies the database is reachable before running any test.
func TestMain(m *testing.M) {
	dsn := os.Getenv("PROCRASTINATOR_TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "PROCRASTINATOR_TEST_DATABASE_URL not set: skipping api tests")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "api: pool:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "api: ping:", err)
		pool.Close()
		os.Exit(1)
	}
	pool.Close()

	os.Exit(m.Run())
}

// testEnv is a fully isolated stack: private schema + migrated pool, fake LLM
// server, file storage, ingest service, and the API server under test.
type testEnv struct {
	handler http.Handler
	schema  string
	pool    *pgxpool.Pool
	llm     *llmState
}

// setLLMPayload re-points the fake LLM's next reply.
func (e *testEnv) setLLMPayload(p string) {
	e.llm.setPayload(p)
}

// envOpts configures newEnv.
type envOpts struct {
	maxBytes int64
	// llmPayload is the extraction JSON returned as the LLM content.
	llmPayload string
	// llmStatus is the HTTP status returned by the fake LLM (default 200).
	llmStatus int
}

// llmState is the mutable fake-LLM behavior shared with the per-test fake
// server. Tests may re-point the payload between uploads (see
// TestListDocuments/Ordered) while the env's handler stays live.
type llmState struct {
	mu      sync.Mutex
	payload string
	status  int
	calls   int
}

// handler returns the http.Handler serving the fake LLM endpoint.
func (s *llmState) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.calls++
		payload, status := s.payload, s.status
		s.mu.Unlock()

		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"llm outage"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		content, _ := json.Marshal(payload)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, content)
	})
}

// setPayload re-points the fake LLM's next reply.
func (s *llmState) setPayload(p string) {
	s.mu.Lock()
	s.payload = p
	s.mu.Unlock()
}

// newEnv builds an isolated per-test stack and registers cleanup for it.
func newEnv(t *testing.T, opts envOpts) *testEnv {
	t.Helper()

	maxBytes := opts.maxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}
	// Per-test mutable LLM behavior; the Ordered documents test re-points the
	// fake at the AMC payload between the two uploads.
	llmPayload := opts.llmPayload
	if llmPayload == "" {
		llmPayload = happyPayload
	}
	llmStatus := opts.llmStatus
	if llmStatus == 0 {
		llmStatus = http.StatusOK
	}

	// Private schema: p_api_<16 hex> (crypto/rand).
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("read crypto/rand: %v", err)
	}
	schema := "p_api_" + hex.EncodeToString(suffix[:])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(os.Getenv("PROCRASTINATOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	// Drop any leftover schema with the same name (should not happen).
	boot, err := pgxpool.New(ctx, cfg.ConnConfig.ConnString())
	if err != nil {
		t.Fatalf("boot pool: %v", err)
	}
	if _, err := boot.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
		boot.Close()
		t.Fatalf("drop leftover schema: %v", err)
	}
	boot.Close()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		pool.Close()
		t.Fatalf("goose set dialect: %v", err)
	}
	if err := goose.Up(stdlib.OpenDBFromPool(pool), migrationsDir); err != nil {
		pool.Close()
		t.Fatalf("goose up: %v", err)
	}

	state := &llmState{payload: llmPayload, status: llmStatus}
	llmServer := httptest.NewServer(state.handler())

	factory := postgres.NewRepoFactory(pool)
	svc := ingest.New(
		factory,
		llm.NewExtractor(llm.New(llmServer.URL, "test-key", "test-model", 10*time.Second)),
		filestorage.New(t.TempDir()),
		maxBytes,
	)
	srv := New(svc, factory.AssetRepo(), factory.DocumentRepo(), maxBytes)

	t.Cleanup(func() {
		llmServer.Close()
		dropCtx, dCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dCancel()
		if _, err := pool.Exec(dropCtx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		pool.Close()
	})

	return &testEnv{handler: srv.Routes(), schema: schema, pool: pool, llm: state}
}

// do performs an HTTP request against the handler via httptest.NewRecorder
// and returns the recorded response. A nil body is passed as a nil io.Reader
// (not a typed-nil *bytes.Buffer) because Go 1.27's httptest.NewRequest
// dereferences *bytes.Buffer bodies without a nil check.
func do(t *testing.T, handler http.Handler, method, path, tenant string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
	if tenant != "" {
		req.Header.Set("X-Tenant-ID", tenant)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// buildMultipart writes a single file part with the given name, filename,
// content type, and content.
func buildMultipart(t *testing.T, name, filename, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name=%q; filename=%q`, name, filename)},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

// pdfBytes is a minimal byte slice sniffed as application/pdf by filestorage.
func pdfBytes(n int) []byte {
	out := append([]byte("%PDF-1.4\nfake invoice body\n"), bytes.Repeat([]byte("x"), n)...)
	out = append(out, []byte("%%EOF")...)
	return out
}

// uploadFile posts a multipart file to /api/documents and returns the
// recorded response plus the decoded asset (when the response is asset JSON).
func (e *testEnv) uploadFile(t *testing.T, tenant string, filename string, contentType string, content []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body, ct := buildMultipart(t, "file", filename, contentType, content)
	rec := do(t, e.handler, http.MethodPost, "/api/documents", tenant, body, ct)

	var asset map[string]any
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &asset); err != nil {
			t.Fatalf("unmarshal upload asset JSON: %v (body: %s)", err, rec.Body.String())
		}
	}
	return rec, asset
}

// strVal returns the string value of key k in m ("" when absent).
func strVal(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// assertErrorEnvelope checks the response body is a JSON object with a single
// non-empty "error" string key.
func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("error body is not a JSON object: %v (body: %q)", err, rec.Body.String())
	}
	if len(env) != 1 {
		t.Fatalf("error body has %d keys, want exactly 1: %q", len(env), rec.Body.String())
	}
	raw, ok := env["error"]
	if !ok {
		t.Fatalf("error body missing key \"error\": %q", rec.Body.String())
	}
	var msg string
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("error body key \"error\" is not a string: %v", err)
	}
	if msg == "" {
		t.Fatalf("error body key \"error\" is empty: %q", rec.Body.String())
	}
}

// TestUpload exercises POST /api/documents end to end.
func TestUpload(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec, asset := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(24))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strVal(asset, "id"); got == "" {
			t.Error("asset id is empty, want non-empty")
		}
		if got := strVal(asset, "brand"); got != "Samsung" {
			t.Errorf("brand = %q, want %q", got, "Samsung")
		}
		if got := strVal(asset, "model"); got != "WF80A" {
			t.Errorf("model = %q, want %q", got, "WF80A")
		}
		if got := strVal(asset, "serial_number"); got != "WM-2024-001" {
			t.Errorf("serial_number = %q, want %q", got, "WM-2024-001")
		}
		if got := strVal(asset, "doc_type"); got != "invoice" {
			t.Errorf("doc_type = %q, want %q", got, "invoice")
		}
		if got := strVal(asset, "currency"); got != "INR" {
			t.Errorf("currency = %q, want %q", got, "INR")
		}
		meta, ok := asset["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("metadata is not a JSON object: %v", asset["metadata"])
		}
		if got := strVal(meta, "invoice_number"); got != "INVNAG2302754" {
			t.Errorf("metadata.invoice_number = %q, want %q", got, "INVNAG2302754")
		}
		if got := strVal(meta, "customer_name"); got != "ACME CORP" {
			t.Errorf("metadata.customer_name = %q, want %q", got, "ACME CORP")
		}
		// Price must serialize as a JSON string with the exact decimal.
		if !strings.Contains(rec.Body.String(), `"price":"39999.99"`) {
			t.Errorf("raw body missing %q (price must be a JSON string): %s", `"price":"39999.99"`, rec.Body.String())
		}
	})

	t.Run("MissingFileField", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.WriteField("notes", "no file attached"); err != nil {
			t.Fatalf("write field: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close writer: %v", err)
		}

		rec := do(t, e.handler, http.MethodPost, "/api/documents", "test-tenant", &buf, w.FormDataContentType())
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingTenant", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		uuid := "00000000-0000-4000-8000-000000000000"
		body, ct := buildMultipart(t, "file", "invoice.pdf", "application/pdf", pdfBytes(16))

		cases := []struct {
			name   string
			method string
			path   string
			body   *bytes.Buffer
			cType  string
		}{
			{name: "POST /api/documents", method: http.MethodPost, path: "/api/documents", body: body, cType: ct},
			{name: "GET /api/assets", method: http.MethodGet, path: "/api/assets"},
			{name: "GET /api/assets/{id}", method: http.MethodGet, path: "/api/assets/" + uuid},
			{name: "GET /api/assets/{id}/documents", method: http.MethodGet, path: "/api/assets/" + uuid + "/documents"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := do(t, e.handler, tc.method, tc.path, "", tc.body, tc.cType)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
				}
				assertErrorEnvelope(t, rec)
			})
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec, _ := e.uploadFile(t, "test-tenant", "notes.txt", "text/plain", []byte("hello world"))
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("Oversize", func(t *testing.T) {
		e := newEnv(t, envOpts{maxBytes: 64})
		rec, _ := e.uploadFile(t, "test-tenant", "big.pdf", "application/pdf", pdfBytes(200))
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("LLMOutage", func(t *testing.T) {
		e := newEnv(t, envOpts{llmStatus: http.StatusInternalServerError})
		rec, _ := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(16))
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("NoIdentity", func(t *testing.T) {
		e := newEnv(t, envOpts{llmPayload: noIdentityPayload})
		rec, _ := e.uploadFile(t, "test-tenant", "other.pdf", "application/pdf", pdfBytes(16))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListAssets exercises GET /api/assets.
func TestListAssets(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/assets", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("WithData", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}

		rec := do(t, e.handler, http.MethodGet, "/api/assets", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"price":"39999.99"`) {
			t.Errorf("raw body missing %q (price must be a JSON string): %s", `"price":"39999.99"`, rec.Body.String())
		}

		var assets []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &assets); err != nil {
			t.Fatalf("unmarshal assets: %v (body: %s)", err, rec.Body.String())
		}
		if len(assets) != 1 {
			t.Fatalf("asset count = %d, want 1 (body: %s)", len(assets), rec.Body.String())
		}
		if got := strVal(assets[0], "id"); got != strVal(asset, "id") {
			t.Errorf("listed asset id = %q, want uploaded asset id %q", got, strVal(asset, "id"))
		}
	})
}

// TestGetAsset exercises GET /api/assets/{id}.
func TestGetAsset(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}

		rec := do(t, e.handler, http.MethodGet, "/api/assets/"+strVal(asset, "id"), "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal asset: %v (body: %s)", err, rec.Body.String())
		}
		if v := strVal(got, "serial_number"); v != "WM-2024-001" {
			t.Errorf("serial_number = %q, want %q", v, "WM-2024-001")
		}
		if v := strVal(got, "doc_type"); v != "invoice" {
			t.Errorf("doc_type = %q, want %q", v, "invoice")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/assets/00000000-0000-4000-8000-000000000000", "test-tenant", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListDocuments exercises GET /api/assets/{id}/documents.
func TestListDocuments(t *testing.T) {
	t.Parallel()

	t.Run("Ordered", func(t *testing.T) {
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"ORD-1","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","metadata":{}}`,
		})

		up1, asset := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up1.Code != http.StatusCreated {
			t.Fatalf("upload 1 status = %d, want 201 (body: %s)", up1.Code, up1.Body.String())
		}

		// Second upload through the SAME env (same schema) with the fake LLM
		// re-pointed at the AMC payload for the SAME serial, so identity
		// resolution links it to the same asset. The 10ms gap keeps the
		// document created_at values (and ordering) distinct.
		time.Sleep(10 * time.Millisecond)
		e.setLLMPayload(`{"classification":"amc","serial_number":"ORD-1","metadata":{"amc_card_number":"AMC-777"}}`)
		up2, asset2 := e.uploadFile(t, "test-tenant", "amc.pdf", "application/pdf", pdfBytes(16))
		if up2.Code != http.StatusCreated {
			t.Fatalf("upload 2 status = %d, want 201 (body: %s)", up2.Code, up2.Body.String())
		}
		if id2 := strVal(asset2, "id"); id2 != strVal(asset, "id") {
			t.Fatalf("upload 2 asset id = %q, want same asset id %q (same serial links to one asset)", id2, strVal(asset, "id"))
		}

		rec := do(t, e.handler, http.MethodGet, "/api/assets/"+strVal(asset, "id")+"/documents", "test-tenant", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var docs []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal documents: %v (body: %s)", err, rec.Body.String())
		}
		if len(docs) != 2 {
			t.Fatalf("document count = %d, want 2 (body: %s)", len(docs), rec.Body.String())
		}
		if got := strVal(docs[0], "doc_type"); got != "invoice" {
			t.Errorf("entry 0 doc_type = %q, want %q", got, "invoice")
		}
		if got := strVal(docs[1], "doc_type"); got != "amc" {
			t.Errorf("entry 1 doc_type = %q, want %q", got, "amc")
		}
		for i, d := range docs {
			if strVal(d, "source_filename") == "" {
				t.Errorf("entry %d source_filename is empty", i)
			}
			if _, ok := d["source_uploaded_at"].(string); !ok || d["source_uploaded_at"] == "" {
				t.Errorf("entry %d source_uploaded_at missing or empty: %v", i, d["source_uploaded_at"])
			}
			if _, ok := d["created_at"].(string); !ok || d["created_at"] == "" {
				t.Errorf("entry %d created_at missing or empty: %v", i, d["created_at"])
			}
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/assets/00000000-0000-4000-8000-000000000000/documents", "test-tenant", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestTenantIsolation verifies that data is fully tenant-scoped: one tenant's
// assets and documents are invisible to another tenant, and the same serial
// uploaded by two tenants creates two distinct assets.
func TestTenantIsolation(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{
		llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"ISO-1","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","metadata":{}}`,
	})

	// Tenant A uploads serial ISO-1.
	upA, assetA := e.uploadFile(t, "test-tenant", "invoice.pdf", "application/pdf", pdfBytes(16))
	if upA.Code != http.StatusCreated {
		t.Fatalf("tenant A upload status = %d, want 201 (body: %s)", upA.Code, upA.Body.String())
	}
	idA := strVal(assetA, "id")
	if idA == "" {
		t.Fatalf("tenant A asset id is empty")
	}

	// Tenant B sees no assets and cannot read tenant A's asset or documents.
	rec := do(t, e.handler, http.MethodGet, "/api/assets", "test-tenant-b", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant B list status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("tenant B list body = %q, want exactly %q (never null)", got, "[]")
	}

	rec = do(t, e.handler, http.MethodGet, "/api/assets/"+idA, "test-tenant-b", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tenant B get asset status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	rec = do(t, e.handler, http.MethodGet, "/api/assets/"+idA+"/documents", "test-tenant-b", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tenant B list documents status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	// Tenant B uploads the SAME serial: a distinct asset must be created.
	upB, assetB := e.uploadFile(t, "test-tenant-b", "invoice-b.pdf", "application/pdf", pdfBytes(16))
	if upB.Code != http.StatusCreated {
		t.Fatalf("tenant B upload status = %d, want 201 (body: %s)", upB.Code, upB.Body.String())
	}
	idB := strVal(assetB, "id")
	if idB == "" {
		t.Fatalf("tenant B asset id is empty")
	}
	if idB == idA {
		t.Fatalf("tenant B asset id %q equals tenant A asset id %q, want distinct assets across tenants", idB, idA)
	}
}

// Compile-time guards: these references must resolve so the RED failure is a
// clean "undefined: New" compile error, not a module-resolution error.
var (
	_ = ingest.ErrTooLarge
	_ data.AssetRepository
	_ data.DocumentRepository
)
