// Package api integration tests for the HTTP API surface:
//
//	api.New(svc, assets, docs, maxBytes) *api.Server
//	(*api.Server).Routes() http.Handler
//
// Endpoints (user id comes from the /api/users/{userId}/... path):
//
//	POST /api/users/{userId}/documents              multipart upload, field "file"
//	GET  /api/users/{userId}/assets                 list assets for the user
//	GET  /api/users/{userId}/assets/{assetId}       single asset
//	GET  /api/users/{userId}/assets/{assetId}/documents  documents for one asset
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

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/review"
	"procrastinator-backend/core/search"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
	"procrastinator-backend/infra/llm"
	"procrastinator-backend/infra/pdftext"
	"procrastinator-backend/infra/postgres"
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
	factory *repo.Factory
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
	// maxStatementLines caps statement import lines (default 100000); 0 means default.
	maxStatementLines int
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

	// Surface the underlying error in the 500 fallback envelope body so test
	// failures are diagnosable (off by default; restored after the test).
	t.Cleanup(func() { responseErrorDebug = false })
	responseErrorDebug = true

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
	// Private-ONLY search_path (no ",public" fallback). With the fallback,
	// goose.Up finds the pre-existing public.goose_db_version (migrated by the
	// dev server), concludes the schema is already migrated, and skips creating
	// tables in the private schema; every test then reads/writes the shared
	// public tables, leaking data across runs. Private-only forces goose to
	// migrate this schema in isolation (tables + the 00002 test Users).
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

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

	factory := postgres.NewFactory(pool)
	movRepo := postgres.NewMovementRepository(pool)
	ledgerSvc := ledger.New(factory, movRepo)
	reviewSvc := review.New(factory)
	searchSvc := search.New(factory.Search)
	svc := ingest.New(
		factory,
		llm.NewExtractor(llm.New(llmServer.URL, "test-key", "test-model", 10*time.Second)),
		filestorage.New(t.TempDir()),
		maxBytes,
		0.7, // default review threshold
		reviewSvc,
	)
	stmtMaxLines := opts.maxStatementLines
	if stmtMaxLines == 0 {
		stmtMaxLines = 100000
	}
	statementSvc := statement.New(
		factory,
		filestorage.NewStatement(t.TempDir()),
		movRepo,
		postgres.NewDocumentRepository(pool),
		pdftext.New(),
		maxBytes, // the statement size limit follows the env's upload limit (oversize tests use maxBytes: 64)
		stmtMaxLines,
	)
	srv := New(svc, factory, ledgerSvc, movRepo, maxBytes, statementSvc, maxBytes, household.New(factory), searchSvc, reviewSvc)

	t.Cleanup(func() {
		llmServer.Close()
		dropCtx, dCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dCancel()
		if _, err := pool.Exec(dropCtx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		pool.Close()
	})

	return &testEnv{handler: srv.Routes(), schema: schema, pool: pool, factory: factory, llm: state}
}

// do performs an HTTP request against the handler via httptest.NewRecorder
// and returns the recorded response. A nil body is passed as a nil io.Reader
// (not a typed-nil *bytes.Buffer) because Go 1.27's httptest.NewRequest
// dereferences *bytes.Buffer bodies without a nil check.
//
// When userID is non-empty and the path starts with /api/finance, the path is
// folded to /api/users/{userID}/finance/... so that legacy call sites keep
// working.
func do(t *testing.T, handler http.Handler, method, path, userID string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	if userID != "" && strings.HasPrefix(path, "/api/finance") {
		path = "/api/users/" + userID + "/finance" + strings.TrimPrefix(path, "/api/finance")
	}
	var reader io.Reader
	if body != nil {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
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

// uploadFile posts a multipart file to /api/users/{userID}/documents and
// returns the recorded response plus the decoded asset (when the response is
// asset JSON).
func (e *testEnv) uploadFile(t *testing.T, userID string, filename string, contentType string, content []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body, ct := buildMultipart(t, "file", filename, contentType, content)
	rec := do(t, e.handler, http.MethodPost, "/api/users/"+userID+"/documents", "", body, ct)

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

// TestUpload exercises POST /api/users/{userId}/documents end to end.
func TestUpload(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(24))

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
		// Personal uploads carry no owner_household_id.
		if _, present := asset["owner_household_id"]; present {
			t.Errorf("owner_household_id present for a personal upload, want absent (body: %s)", rec.Body.String())
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

		rec := do(t, e.handler, http.MethodPost, "/api/users/test-user/documents", "", &buf, w.FormDataContentType())
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("MissingUser", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		uuid := "00000000-0000-4000-8000-000000000000"
		body, ct := buildMultipart(t, "file", "invoice.pdf", "application/pdf", pdfBytes(16))

		// Empty {userId} path segment: chi still routes these to the
		// {userId} pattern, and the middleware must reject them with 400
		// ("missing userId").
		cases := []struct {
			name   string
			method string
			path   string
			body   *bytes.Buffer
			cType  string
		}{
			{name: "POST /api/users//documents", method: http.MethodPost, path: "/api/users//documents", body: body, cType: ct},
			{name: "GET /api/users//assets", method: http.MethodGet, path: "/api/users//assets"},
			{name: "GET /api/users//assets/{id}", method: http.MethodGet, path: "/api/users//assets/" + uuid},
			{name: "GET /api/users//assets/{id}/documents", method: http.MethodGet, path: "/api/users//assets/" + uuid + "/documents"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := do(t, e.handler, tc.method, tc.path, "", tc.body, tc.cType)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
				}
				assertErrorEnvelope(t, rec)
			})
		}
	})

	t.Run("MalformedUser", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		uuid := "00000000-0000-4000-8000-000000000000"
		// Malformed {userId} path params (bad character sets are
		// percent-encoded so they stay a single path segment).
		cases := []struct {
			name   string
			userID string
		}{
			{name: "65 chars", userID: strings.Repeat("a", 65)},
			{name: "bad char slash", userID: "bad%2Fuser"},
			{name: "bad char space", userID: "bad%20user"},
		}
		routes := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/users/{userId}/assets"},
			{http.MethodGet, "/api/users/{userId}/assets/" + uuid},
			{http.MethodGet, "/api/users/{userId}/assets/" + uuid + "/documents"},
			{http.MethodPost, "/api/users/{userId}/documents"},
		}
		for _, tc := range cases {
			for _, ep := range routes {
				path := strings.ReplaceAll(ep.path, "/api/users/{userId}", "/api/users/"+tc.userID)
				t.Run(tc.name+" "+ep.method+" "+ep.path, func(t *testing.T) {
					rec := do(t, e.handler, ep.method, path, "", nil, "")
					if rec.Code != http.StatusBadRequest {
						t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
					}
					assertErrorEnvelope(t, rec)
				})
			}
		}
	})

	t.Run("UnregisteredUser", func(t *testing.T) {
		e := newEnv(t, envOpts{})

		uuid := "00000000-0000-4000-8000-000000000000"
		routes := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/users/unknown-user/assets"},
			{http.MethodGet, "/api/users/unknown-user/assets/" + uuid},
			{http.MethodGet, "/api/users/unknown-user/assets/" + uuid + "/documents"},
			{http.MethodPost, "/api/users/unknown-user/documents"},
		}
		for _, ep := range routes {
			t.Run(ep.method+" "+ep.path, func(t *testing.T) {
				rec := do(t, e.handler, ep.method, ep.path, "", nil, "")
				if rec.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
				}
				assertErrorEnvelope(t, rec)
			})
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec, _ := e.uploadFile(t, "test-user", "notes.txt", "text/plain", []byte("hello world"))
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("Oversize", func(t *testing.T) {
		e := newEnv(t, envOpts{maxBytes: 64})
		rec, _ := e.uploadFile(t, "test-user", "big.pdf", "application/pdf", pdfBytes(200))
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("LLMOutage", func(t *testing.T) {
		e := newEnv(t, envOpts{llmStatus: http.StatusInternalServerError})
		rec, _ := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("NoIdentity", func(t *testing.T) {
		e := newEnv(t, envOpts{llmPayload: noIdentityPayload})
		rec, _ := e.uploadFile(t, "test-user", "other.pdf", "application/pdf", pdfBytes(16))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("HouseholdScope", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		ctx := context.Background()
		hh, err := e.factory.Households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("create household: %v", err)
		}
		if err := e.factory.Households.AddMember(ctx, hh.ID, "test-user", repo.Owner("test-user")); err != nil {
			t.Fatalf("add member: %v", err)
		}
		rec, asset := e.uploadWithOwnerHousehold(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16), hh.ID)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strVal(asset, "owner_household_id"); got != hh.ID {
			t.Errorf("owner_household_id = %q, want %q", got, hh.ID)
		}
	})

	t.Run("NonMemberScope", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		ctx := context.Background()
		hh, err := e.factory.Households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("create household: %v", err)
		}
		if err := e.factory.Households.AddMember(ctx, hh.ID, "test-user", repo.Owner("test-user")); err != nil {
			t.Fatalf("add member: %v", err)
		}
		rec, _ := e.uploadWithOwnerHousehold(t, "test-user-b", "invoice.pdf", "application/pdf", pdfBytes(16), hh.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// TestUploadDocument_LowConfidenceHolds verifies that a document whose
// extraction confidence falls below the 0.7 threshold is held for review
// (202) with a pending IngestReview rather than auto-committed.
func TestUploadDocument_LowConfidenceHolds(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	// Low-confidence payload (0.3 < 0.7 threshold) with valid identity fields.
	e.llm.setPayload(`{"classification":"invoice","brand":"Test","model":"M1","serial_number":"SN-LOW-1","confidence":0.3}`)

	body, ct := buildMultipart(t, "file", "low.pdf", "application/pdf", pdfBytes(16))
	rec := do(t, e.handler, http.MethodPost, "/api/users/alice/documents", "", body, ct)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}

	var review gen.IngestReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("unmarshal ingest review: %v (body: %s)", err, rec.Body.String())
	}
	if review.Id == "" {
		t.Errorf("review id is empty, want non-empty (body: %s)", rec.Body.String())
	}
	if review.State != "pending" {
		t.Errorf("review state = %q, want %q (body: %s)", review.State, "pending", rec.Body.String())
	}
}

// uploadWithOwnerHousehold posts a multipart file with an owner_household_id
// form field to /api/users/{userID}/documents.
func (e *testEnv) uploadWithOwnerHousehold(t *testing.T, userID, filename, contentType string, content []byte, ownerHouseholdID string) (*httptest.ResponseRecorder, map[string]any) {
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
	if err := w.WriteField("owner_household_id", ownerHouseholdID); err != nil {
		t.Fatalf("write owner_household_id field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	rec := do(t, e.handler, http.MethodPost, "/api/users/"+userID+"/documents", "", &buf, w.FormDataContentType())
	var asset map[string]any
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &asset); err != nil {
			t.Fatalf("unmarshal upload asset JSON: %v (body: %s)", err, rec.Body.String())
		}
	}
	return rec, asset
}

// TestRouteTable verifies that every registered route returns a JSON response
// (Content-Type contains application/json) when reached, and that the old
// /api/finance/... paths are no longer routable.
func TestRouteTable(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{})

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/users/test-user/documents"},
		{http.MethodGet, "/api/users/test-user/assets"},
		{http.MethodGet, "/api/users/test-user/assets/" + unknownUUID},
		{http.MethodGet, "/api/users/test-user/assets/" + unknownUUID + "/documents"},
		{http.MethodPost, "/api/users/test-user/finance/accounts"},
		{http.MethodGet, "/api/users/test-user/finance/accounts"},
		{http.MethodGet, "/api/users/test-user/finance/accounts/" + unknownUUID},
		{http.MethodPost, "/api/users/test-user/finance/movements"},
		{http.MethodGet, "/api/users/test-user/finance/movements"},
		{http.MethodGet, "/api/users/test-user/finance/movements/" + unknownUUID},
		{http.MethodPatch, "/api/users/test-user/finance/movements/" + unknownUUID},
		{http.MethodDelete, "/api/users/test-user/finance/movements/" + unknownUUID},
		{http.MethodPost, "/api/users/test-user/finance/movements/" + unknownUUID + "/link"},
		{http.MethodDelete, "/api/users/test-user/finance/movements/" + unknownUUID + "/link"},
		{http.MethodPost, "/api/users/test-user/finance/import-batches"},
		{http.MethodGet, "/api/users/test-user/finance/import-batches"},
		{http.MethodGet, "/api/users/test-user/finance/import-batches/" + unknownUUID},
		{http.MethodPost, "/api/users/test-user/finance/import-batches/" + unknownUUID + "/commit"},
		{http.MethodPost, "/api/users/test-user/finance/import-batches/" + unknownUUID + "/discard"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := do(t, e.handler, rt.method, rt.path, "", nil, "")
			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json (status %d)", ct, rec.Code)
			}
		})
	}

	legacy := []string{
		"/api/finance/accounts",
		"/api/finance/movements",
		"/api/finance/import-batches",
	}
	for _, path := range legacy {
		t.Run("legacy "+path, func(t *testing.T) {
			rec := do(t, e.handler, http.MethodGet, path, "", nil, "")
			ct := rec.Header().Get("Content-Type")
			if strings.Contains(ct, "application/json") {
				t.Errorf("legacy path %q returned Content-Type %q, want no JSON (status %d)", path, ct, rec.Code)
			}
		})
	}
}

// TestListAssets exercises GET /api/users/{userId}/assets.
func TestListAssets(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want exactly %q (never null)", got, "[]")
		}
	})

	t.Run("WithData", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}

		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets", "", nil, "")
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

// TestGetAsset exercises GET /api/users/{userId}/assets/{assetId}.
func TestGetAsset(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		up, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
		}

		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+strVal(asset, "id"), "", nil, "")
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
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000", "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestListDocuments exercises GET /api/users/{userId}/assets/{assetId}/documents.
func TestListDocuments(t *testing.T) {
	t.Parallel()

	t.Run("Ordered", func(t *testing.T) {
		e := newEnv(t, envOpts{
			llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"ORD-1","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","metadata":{}}`,
		})

		up1, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
		if up1.Code != http.StatusCreated {
			t.Fatalf("upload 1 status = %d, want 201 (body: %s)", up1.Code, up1.Body.String())
		}

		// Second upload through the SAME env (same schema) with the fake LLM
		// re-pointed at the AMC payload for the SAME serial, so identity
		// resolution links it to the same asset. The 10ms gap keeps the
		// document created_at values (and ordering) distinct.
		time.Sleep(10 * time.Millisecond)
		e.setLLMPayload(`{"classification":"amc","serial_number":"ORD-1","metadata":{"amc_card_number":"AMC-777"}}`)
		up2, asset2 := e.uploadFile(t, "test-user", "amc.pdf", "application/pdf", pdfBytes(16))
		if up2.Code != http.StatusCreated {
			t.Fatalf("upload 2 status = %d, want 201 (body: %s)", up2.Code, up2.Body.String())
		}
		if id2 := strVal(asset2, "id"); id2 != strVal(asset, "id") {
			t.Fatalf("upload 2 asset id = %q, want same asset id %q (same serial links to one asset)", id2, strVal(asset, "id"))
		}

		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+strVal(asset, "id")+"/documents", "", nil, "")
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
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/00000000-0000-4000-8000-000000000000/documents", "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})
}

// TestUserIsolation verifies that data is fully user-scoped: one user's
// assets and documents are invisible to another user, and the same serial
// uploaded by two Users creates two distinct assets.
func TestUserIsolation(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{
		llmPayload: `{"classification":"invoice","brand":"Samsung","model":"WF80A","serial_number":"ISO-1","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","metadata":{}}`,
	})

	// User A uploads serial ISO-1.
	upA, assetA := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
	if upA.Code != http.StatusCreated {
		t.Fatalf("user A upload status = %d, want 201 (body: %s)", upA.Code, upA.Body.String())
	}
	idA := strVal(assetA, "id")
	if idA == "" {
		t.Fatalf("user A asset id is empty")
	}

	// User B sees no assets and cannot read user A's asset or documents.
	rec := do(t, e.handler, http.MethodGet, "/api/users/test-user-b/assets", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("user B list status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("user B list body = %q, want exactly %q (never null)", got, "[]")
	}

	rec = do(t, e.handler, http.MethodGet, "/api/users/test-user-b/assets/"+idA, "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user B get asset status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	rec = do(t, e.handler, http.MethodGet, "/api/users/test-user-b/assets/"+idA+"/documents", "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user B list documents status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	// User B uploads the SAME serial: a distinct asset must be created.
	upB, assetB := e.uploadFile(t, "test-user-b", "invoice-b.pdf", "application/pdf", pdfBytes(16))
	if upB.Code != http.StatusCreated {
		t.Fatalf("user B upload status = %d, want 201 (body: %s)", upB.Code, upB.Body.String())
	}
	idB := strVal(assetB, "id")
	if idB == "" {
		t.Fatalf("user B asset id is empty")
	}
	if idB == idA {
		t.Fatalf("user B asset id %q equals user A asset id %q, want distinct assets across Users", idB, idA)
	}
}

// TestScopeAccess verifies the scope access contract: a user sees their
// personal rows plus the rows of households they are a member of, and nothing
// else.
func TestScopeAccess(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	ctx := context.Background()

	// Household owned by test-user, with test-user as a member.
	hh, err := e.factory.Households.Create(ctx, entity.Household{DisplayName: "H1"}, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create household: %v", err)
	}
	if hh.ID == "" {
		t.Fatal("household id is empty")
	}
	if err := e.factory.Households.AddMember(ctx, hh.ID, "test-user", repo.Owner("test-user")); err != nil {
		t.Fatalf("add household member: %v", err)
	}

	// Household-scoped asset owned by the household.
	hhAsset, err := e.factory.Assets.Create(ctx, entity.Asset{
		DocType:          "invoice",
		OwnerHouseholdID: &hh.ID,
	}, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create household asset: %v", err)
	}
	if hhAsset.ID == "" {
		t.Fatal("household asset id is empty")
	}

	// Personal asset for test-user (via the HTTP upload).
	up, personal := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(16))
	if up.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201 (body: %s)", up.Code, up.Body.String())
	}
	personalID := strVal(personal, "id")
	if personalID == "" {
		t.Fatal("personal asset id is empty")
	}

	// listIDs decodes the asset list body and returns the set of asset ids.
	listIDs := func(t *testing.T, path string) map[string]bool {
		t.Helper()
		rec := do(t, e.handler, http.MethodGet, path, "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200 (body: %s)", path, rec.Code, rec.Body.String())
		}
		var assets []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &assets); err != nil {
			t.Fatalf("unmarshal asset list: %v (body: %s)", err, rec.Body.String())
		}
		ids := make(map[string]bool, len(assets))
		for _, a := range assets {
			ids[strVal(a, "id")] = true
		}
		return ids
	}

	t.Run("PersonalVisible", func(t *testing.T) {
		ids := listIDs(t, "/api/users/test-user/assets")
		if !ids[personalID] {
			t.Errorf("personal asset %q not visible to its owner (ids: %v)", personalID, ids)
		}
	})

	t.Run("HouseholdVisibleWhenMember", func(t *testing.T) {
		ids := listIDs(t, "/api/users/test-user/assets")
		if !ids[hhAsset.ID] {
			t.Errorf("household asset %q not visible to owner+member (ids: %v)", hhAsset.ID, ids)
		}
	})

	t.Run("AnotherUsersPersonalInvisible", func(t *testing.T) {
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user-b/assets/"+personalID, "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("NonMemberHouseholdInvisible", func(t *testing.T) {
		ids := listIDs(t, "/api/users/test-user-b/assets")
		if ids[hhAsset.ID] {
			t.Errorf("household asset %q visible to non-member (ids: %v)", hhAsset.ID, ids)
		}
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user-b/assets/"+hhAsset.ID, "", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// Compile-time guards: these references must resolve so the RED failure is a
// clean "undefined: New" compile error, not a module-resolution error.
var (
	_ = ingest.ErrTooLarge
	_ = repo.ErrNotFound
)
