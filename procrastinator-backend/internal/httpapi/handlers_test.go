// End-to-end HTTP handler tests over a real PostgreSQL store + fake LLM httptest server,
// config injected, and each test boots on a private per-test schema so everything runs with t.Parallel().
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/ingest"
	"procrastinator-backend/internal/llm"
	"procrastinator-backend/internal/store"
)

var testDSN string

func TestMain(m *testing.M) {
	testDSN = os.Getenv("LM_TEST_DATABASE_URL")
	os.Exit(m.Run())
}

var schemaSeq int64

func dsnForSchema(base, schema string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func migrationsDir() string {
	return filepath.Join("..", "..", "migrations")
}

func newTestServer(t *testing.T, llmFake *httptest.Server, maxBytes int64) (*httptest.Server, *store.Store) {
	t.Helper()

	if testDSN == "" {
		t.Skip("LM_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("lm_httpapi_%d", atomic.AddInt64(&schemaSeq, 1))

	bootstrapPool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("failed to create bootstrap pool: %v", err)
	}
	_, err = bootstrapPool.Exec(ctx, "CREATE SCHEMA "+schema)
	if err != nil {
		t.Fatalf("failed to create schema %s: %v", schema, err)
	}
	bootstrapPool.Close()

	t.Cleanup(func() {
		bootstrapPool, err := pgxpool.New(ctx, testDSN)
		if err != nil {
			t.Logf("cleanup: failed to create bootstrap pool: %v", err)
			return
		}
		defer bootstrapPool.Close()
		_, err = bootstrapPool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		if err != nil {
			t.Logf("cleanup: failed to drop schema %s: %v", schema, err)
		}
	})

	st, err := store.Open(ctx, dsnForSchema(testDSN, schema))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	err = st.Migrate(ctx, migrationsDir())
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	cfg := config.Config{
		LLMBaseURL:     llmFake.URL,
		LLMAPIKey:      "test-api-key",
		LLMModel:       "test-model",
		StorageDir:     t.TempDir(),
		MaxUploadBytes: maxBytes,
		LLMTimeout:     5 * time.Second,
	}
	if maxBytes == 0 {
		cfg.MaxUploadBytes = 1 << 20 // 1MB default
	}

	var extractor ingest.Extractor = llm.New(cfg)
	srv := New(cfg, st.Pool(), extractor)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	return ts, st
}

type fakeLLMResponse struct {
	status  int
	content string
}

func newFakeLLM(t *testing.T, responses ...fakeLLMResponse) *httptest.Server {
	t.Helper()

	if len(responses) == 0 {
		responses = append(responses, fakeLLMResponse{0, ""})
	}

	var mu sync.Mutex
	idx := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		mu.Lock()
		resp := responses[idx]
		if idx < len(responses)-1 {
			idx++
		}
		mu.Unlock()

		if resp.status == 0 { // Success (200 OK)
			w.Header().Set("Content-Type", "application/json")
			b, err := json.Marshal(resp.content)
			if err != nil {
				t.Fatalf("failed to marshal LLM content: %v", err)
			}
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, b)
		} else { // LLM failure simulation
			w.WriteHeader(resp.status)
			fmt.Fprint(w, `{"error":"fake llm failure"}`)
		}
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

var pdfFixture = []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n%%EOF")
var pngFixture = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0x0D, 'I', 'H', 'D', 'R', 0}
var textFixture = []byte("just some plain text, definitely not a document\n")

var invoiceContent = `{"document_type":"invoice","brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-200","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`
var warrantyContent = `{"document_type":"warranty","serial_number":"sn-200","warranty_end":"2028-12-31"}`
var noIdentityContent = `{"document_type":"other"}`
var appleInvoiceContent = `{"document_type":"invoice","brand":"Apple","model":"MacBook Pro","serial_number":"SN-300","price":"39999.99","currency":"INR"}`

func postMultipart(t *testing.T, ts *httptest.Server, build func(*multipart.Writer) error) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	err := build(mw)
	if err != nil {
		t.Fatalf("failed to build multipart: %v", err)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/documents", &buf)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to perform request: %v", err)
	}
	return resp
}

func uploadFile(t *testing.T, ts *httptest.Server, filename string, data []byte) *http.Response {
	t.Helper()
	return postMultipart(t, ts, func(mw *multipart.Writer) error {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	})
}

func uploadTextField(t *testing.T, ts *httptest.Server, field, value string) *http.Response {
	t.Helper()
	return postMultipart(t, ts, func(mw *multipart.Writer) error {
		return mw.WriteField(field, value)
	})
}

func doGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("failed to create GET request: %v", err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to perform GET request: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	resp.Body.Close()
	return body
}

func countRows(t *testing.T, st *store.Store, table string) int64 {
	t.Helper()
	var count int64
	err := st.Pool().QueryRow(context.Background(), fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows in table %s: %v", table, err)
	}
	return count
}

type errorEnvelope struct {
	Error string `json:"error"`
}

func wantErrorEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var errEnv errorEnvelope
	err := json.Unmarshal(body, &errEnv)
	if err != nil {
		t.Errorf("failed to unmarshal error envelope: %v, body: %s", err, string(body))
		return
	}
	if errEnv.Error == "" {
		t.Errorf("expected error message in envelope, got empty; body: %s", string(body))
	}
}

func wantAssetJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	err := json.Unmarshal(body, &m)
	if err != nil {
		t.Fatalf("failed to unmarshal asset JSON: %v, body: %s", err, string(body))
	}
	return m
}

func wantStringField(t *testing.T, m map[string]any, field, want string) {
	t.Helper()
	v, ok := m[field].(string)
	if !ok {
		t.Errorf("expected string field %s, got type %T", field, m[field])
		return
	}
	if v != want {
		t.Errorf("field %s: want %q, got %q", field, want, v)
	}
}

func wantID(t *testing.T, m map[string]any) string {
	t.Helper()
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Errorf("expected non-empty string 'id' field, got %v (type %T)", m["id"], m["id"])
	}
	return id
}

func TestHandleUpload(t *testing.T) {
	t.Parallel()

	type uploadCase struct {
		name        string
		filename    string
		data        []byte
		textField   string
		llm         []fakeLLMResponse
		maxBytes    int64
		wantStatus  int
		wantSources int64
		wantAssets  int64
		wantDocs    int64
		wantAsset   bool
	}

	cases := []uploadCase{
		{
			name:        "invoice upload returns 201 with asset json",
			filename:    "invoice.pdf",
			data:        pdfFixture,
			llm:         []fakeLLMResponse{{0, invoiceContent}},
			wantStatus:  201,
			wantSources: 1,
			wantAssets:  1,
			wantDocs:    1,
			wantAsset:   true,
		},
		{
			name:        "missing file field returns 400 and persists nothing",
			textField:   "notes",
			llm:         []fakeLLMResponse{{0, invoiceContent}},
			wantStatus:  400,
			wantSources: 0,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "unsupported content type returns 415 and persists nothing",
			filename:    "notes.txt",
			data:        textFixture,
			llm:         []fakeLLMResponse{{0, invoiceContent}}, // LLM is never reached; harmless
			wantStatus:  415,
			wantSources: 0,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "oversized upload returns 413 and persists nothing",
			filename:    "big.pdf",
			data:        pdfFixture,
			llm:         []fakeLLMResponse{{0, invoiceContent}},
			maxBytes:    32, // smaller than fixture + multipart overhead
			wantStatus:  413,
			wantSources: 0,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "llm outage returns 502 and retains source",
			filename:    "invoice.pdf",
			data:        pdfFixture,
			llm:         []fakeLLMResponse{{500, ""}},
			wantStatus:  502,
			wantSources: 1,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "malformed llm payload returns 502 and retains source",
			filename:    "invoice.pdf",
			data:        pdfFixture,
			llm:         []fakeLLMResponse{{0, "not a json payload"}},
			wantStatus:  502,
			wantSources: 1,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "unidentifiable document returns 422 and retains source",
			filename:    "other.pdf",
			data:        pdfFixture,
			llm:         []fakeLLMResponse{{0, noIdentityContent}},
			wantStatus:  422,
			wantSources: 1,
			wantAssets:  0,
			wantDocs:    0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			llmFake := newFakeLLM(t, tc.llm...)
			ts, st := newTestServer(t, llmFake, tc.maxBytes)

			var resp *http.Response
			if tc.textField != "" {
				resp = uploadTextField(t, ts, tc.textField, "no file here")
			} else {
				resp = uploadFile(t, ts, tc.filename, tc.data)
			}
			body := readBody(t, resp)

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("want status %d, got %d; body: %s", tc.wantStatus, resp.StatusCode, string(body))
			}

			if tc.wantStatus == http.StatusCreated { // 201
				m := wantAssetJSON(t, body)
				if tc.wantAsset {
					wantID(t, m) // just check if it exists and non-empty

					wantStringField(t, m, "brand", "Samsung")
					wantStringField(t, m, "model", "WW90T534DAW")
					wantStringField(t, m, "serial_number", "SN-200")
					wantStringField(t, m, "purchase_date", "2024-03-15")
					wantStringField(t, m, "price", "1299.99") // JSON string, not number
					wantStringField(t, m, "currency", "EUR")
					wantStringField(t, m, "warranty_start", "2024-03-15")
					wantStringField(t, m, "warranty_end", "2026-03-15")

					if _, ok := m["created_at"].(string); !ok || m["created_at"].(string) == "" {
						t.Errorf("expected non-empty string 'created_at' field, got %v (type %T)", m["created_at"], m["created_at"])
					}
					if _, ok := m["updated_at"].(string); !ok || m["updated_at"].(string) == "" {
						t.Errorf("expected non-empty string 'updated_at' field, got %v (type %T)", m["updated_at"], m["updated_at"])
					}
				}
			} else { // 4xx/5xx
				wantErrorEnvelope(t, body)
			}

			if tc.wantSources != -1 {
				actualSources := countRows(t, st, "sources")
				if actualSources != tc.wantSources {
					t.Errorf("want %d sources, got %d", tc.wantSources, actualSources)
				}
			}
			if tc.wantAssets != -1 {
				actualAssets := countRows(t, st, "assets")
				if actualAssets != tc.wantAssets {
					t.Errorf("want %d assets, got %d", tc.wantAssets, actualAssets)
				}
			}
			if tc.wantDocs != -1 {
				actualDocs := countRows(t, st, "documents")
				if actualDocs != tc.wantDocs {
					t.Errorf("want %d documents, got %d", tc.wantDocs, actualDocs)
				}
			}
		})
	}
}

func TestHandleUploadWarrantyMerge(t *testing.T) {
	t.Parallel()

	llmFake := newFakeLLM(t,
		fakeLLMResponse{0, invoiceContent},
		fakeLLMResponse{0, warrantyContent},
	)
	ts, st := newTestServer(t, llmFake, 0)

	// First upload: invoice
	resp1 := uploadFile(t, ts, "invoice.pdf", pdfFixture)
	body1 := readBody(t, resp1)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first upload: want status 201, got %d; body: %s", resp1.StatusCode, string(body1))
	}
	m1 := wantAssetJSON(t, body1)
	id1 := wantID(t, m1)

	// Second upload: warranty, should merge into the same asset
	resp2 := uploadFile(t, ts, "warranty.png", pngFixture)
	body2 := readBody(t, resp2)
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("second upload: want status 201, got %d; body: %s", resp2.StatusCode, string(body2))
	}
	m2 := wantAssetJSON(t, body2)
	id2 := wantID(t, m2)

	if id1 != id2 {
		t.Errorf("expected asset IDs to be identical on merge, got %q and %q", id1, id2)
	}

	// Assert merged fields. The serial's display value follows the latest
	// extraction ("sn-200") — non-empty values overwrite on merge (D4);
	// identity matched on norm_serial, so the case drift is expected.
	wantStringField(t, m2, "serial_number", "sn-200")
	wantStringField(t, m2, "brand", "Samsung")             // retained from invoice
	wantStringField(t, m2, "price", "1299.99")             // retained from invoice (absent in warranty extraction)
	wantStringField(t, m2, "warranty_start", "2024-03-15") // retained from invoice (absent in warranty extraction)
	wantStringField(t, m2, "warranty_end", "2028-12-31")   // overwritten by warranty extraction

	// Check row counts
	actualSources := countRows(t, st, "sources")
	if actualSources != 2 {
		t.Errorf("want 2 sources, got %d", actualSources)
	}
	actualAssets := countRows(t, st, "assets")
	if actualAssets != 1 {
		t.Errorf("want 1 asset, got %d", actualAssets)
	}
	actualDocs := countRows(t, st, "documents")
	if actualDocs != 2 {
		t.Errorf("want 2 documents, got %d", actualDocs)
	}
}

func TestListAssets(t *testing.T) {
	t.Parallel()

	type seedUpload struct {
		filename string
		data     []byte
		content  string
	}

	type listCase struct {
		name    string
		seeds   []seedUpload
		wantLen int
	}

	cases := []listCase{
		{
			name:    "empty registry returns 200 with empty array",
			seeds:   nil,
			wantLen: 0,
		},
		{
			name: "lists all assets with stable order",
			seeds: []seedUpload{
				{"invoice.pdf", pdfFixture, invoiceContent},
				{"macbook.pdf", pdfFixture, appleInvoiceContent},
			},
			wantLen: 2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			llmResponses := make([]fakeLLMResponse, len(tc.seeds))
			for i, s := range tc.seeds {
				llmResponses[i] = fakeLLMResponse{0, s.content}
			}
			llmFake := newFakeLLM(t, llmResponses...)

			ts, _ := newTestServer(t, llmFake, 0)

			for _, s := range tc.seeds {
				resp := uploadFile(t, ts, s.filename, s.data)
				body := readBody(t, resp)
				if resp.StatusCode != http.StatusCreated {
					t.Fatalf("seed upload failed: want status 201, got %d; body: %s", resp.StatusCode, string(body))
				}
			}

			resp := doGet(t, ts, "/api/assets")
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("want status 200, got %d; body: %s", resp.StatusCode, string(body))
			}

			if tc.wantLen == 0 {
				if string(body) != "[]" {
					t.Errorf(`want empty array JSON "[]", got %q`, string(body))
				}
				var assets []map[string]any
				err := json.Unmarshal(body, &assets)
				if err != nil {
					t.Fatalf("failed to unmarshal empty assets JSON: %v", err)
				}
				if len(assets) != 0 {
					t.Errorf("want 0 assets, got %d", len(assets))
				}
			} else {
				var assets []map[string]any
				err := json.Unmarshal(body, &assets)
				if err != nil {
					t.Fatalf("failed to unmarshal assets JSON: %v, body: %s", err, string(body))
				}
				if len(assets) != tc.wantLen {
					t.Fatalf("want %d assets, got %d", tc.wantLen, len(assets))
				}

				ids := make(map[string]struct{})
				for _, asset := range assets {
					id := wantID(t, asset)
					if _, ok := ids[id]; ok {
						t.Errorf("duplicate asset ID found: %s", id)
					}
					ids[id] = struct{}{}
				}

				// Make a second request to ensure stable order
				resp2 := doGet(t, ts, "/api/assets")
				body2 := readBody(t, resp2)
				if resp2.StatusCode != http.StatusOK {
					t.Fatalf("second list request: want status 200, got %d; body: %s", resp2.StatusCode, string(body2))
				}
				if !bytes.Equal(body, body2) {
					t.Errorf("asset order not stable; first: %s, second: %s", string(body), string(body2))
				}

				if tc.wantLen == 2 {
					var appleAsset map[string]any
					for _, asset := range assets {
						if sn, ok := asset["serial_number"].(string); ok && sn == "SN-300" {
							appleAsset = asset
							break
						}
					}
					if appleAsset == nil {
						t.Fatalf("did not find asset with serial_number SN-300")
					}
					wantStringField(t, appleAsset, "price", "39999.99")
					wantStringField(t, appleAsset, "currency", "INR")
				}
			}
		})
	}
}

func TestGetAsset(t *testing.T) {
	t.Parallel()

	type seedUpload struct {
		filename string
		data     []byte
		content  string
	}

	type getCase struct {
		name       string
		seeds      []seedUpload
		id         string // if empty, derived from first seed upload
		wantStatus int
	}

	cases := []getCase{
		{
			name:       "existing asset returns 200",
			seeds:      []seedUpload{{"invoice.pdf", pdfFixture, invoiceContent}},
			id:         "",
			wantStatus: 200,
		},
		{
			name:       "unknown asset returns 404",
			seeds:      nil,
			id:         "00000000-0000-4000-8000-000000000000",
			wantStatus: 404,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			llmResponses := make([]fakeLLMResponse, len(tc.seeds))
			for i, s := range tc.seeds {
				llmResponses[i] = fakeLLMResponse{0, s.content}
			}
			llmFake := newFakeLLM(t, llmResponses...)

			ts, _ := newTestServer(t, llmFake, 0)

			assetID := tc.id
			if len(tc.seeds) > 0 {
				resp := uploadFile(t, ts, tc.seeds[0].filename, tc.seeds[0].data)
				body := readBody(t, resp)
				if resp.StatusCode != http.StatusCreated {
					t.Fatalf("seed upload failed: want status 201, got %d; body: %s", resp.StatusCode, string(body))
				}
				m := wantAssetJSON(t, body)
				assetID = wantID(t, m)
			}

			resp := doGet(t, ts, "/api/assets/"+assetID)
			body := readBody(t, resp)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("want status %d, got %d; body: %s", tc.wantStatus, resp.StatusCode, string(body))
			}

			if tc.wantStatus == http.StatusOK { // 200
				m := wantAssetJSON(t, body)
				wantStringField(t, m, "id", assetID)
				wantStringField(t, m, "serial_number", "SN-200")
			} else { // 4xx
				wantErrorEnvelope(t, body)
			}
		})
	}
}

func TestListAssetDocuments(t *testing.T) {
	t.Parallel()

	type seedUpload struct {
		filename string
		data     []byte
		content  string
	}

	type docExpect struct {
		docType  string
		filename string
	}

	type docsCase struct {
		name        string
		seeds       []seedUpload
		id          string // if empty, derived from first seed upload
		wantStatus  int
		wantEntries int
		wantOrder   []docExpect
	}

	cases := []docsCase{
		{
			name: "documents listed in ingestion order with source metadata",
			seeds: []seedUpload{
				{"invoice.pdf", pdfFixture, invoiceContent},
				{"warranty.png", pngFixture, warrantyContent},
			},
			id:          "",
			wantStatus:  200,
			wantEntries: 2,
			wantOrder: []docExpect{
				{"invoice", "invoice.pdf"},
				{"warranty", "warranty.png"},
			},
		},
		{
			name:        "unknown asset returns 404",
			seeds:       nil,
			id:          "00000000-0000-4000-8000-000000000000",
			wantStatus:  404,
			wantEntries: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			llmResponses := make([]fakeLLMResponse, len(tc.seeds))
			for i, s := range tc.seeds {
				llmResponses[i] = fakeLLMResponse{0, s.content}
			}
			llmFake := newFakeLLM(t, llmResponses...)

			ts, _ := newTestServer(t, llmFake, 0)

			assetID := tc.id
			if len(tc.seeds) > 0 {
				resp := uploadFile(t, ts, tc.seeds[0].filename, tc.seeds[0].data)
				body := readBody(t, resp)
				if resp.StatusCode != http.StatusCreated {
					t.Fatalf("seed upload (first) failed: want status 201, got %d; body: %s", resp.StatusCode, string(body))
				}
				m := wantAssetJSON(t, body)
				assetID = wantID(t, m)

				for i := 1; i < len(tc.seeds); i++ {
					resp = uploadFile(t, ts, tc.seeds[i].filename, tc.seeds[i].data)
					body = readBody(t, resp)
					if resp.StatusCode != http.StatusCreated {
						t.Fatalf("seed upload (%d) failed: want status 201, got %d; body: %s", i, resp.StatusCode, string(body))
					}
				}
			}

			resp := doGet(t, ts, "/api/assets/"+assetID+"/documents")
			body := readBody(t, resp)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("want status %d, got %d; body: %s", tc.wantStatus, resp.StatusCode, string(body))
			}

			if tc.wantStatus == http.StatusOK { // 200
				var docs []map[string]any
				err := json.Unmarshal(body, &docs)
				if err != nil {
					t.Fatalf("failed to unmarshal documents JSON: %v, body: %s", err, string(body))
				}
				if len(docs) != tc.wantEntries {
					t.Fatalf("want %d document entries, got %d", tc.wantEntries, len(docs))
				}

				uploadedAtTimes := make([]time.Time, 0, len(docs))
				for i, doc := range docs {
					id, ok := doc["id"].(string)
					if !ok || id == "" {
						t.Errorf("document entry %d: expected non-empty string 'id' field, got %v (type %T)", i, doc["id"], doc["id"])
					}

					wantStringField(t, doc, "doc_type", tc.wantOrder[i].docType)
					wantStringField(t, doc, "source_filename", tc.wantOrder[i].filename)

					extractedFields, ok := doc["extracted_fields"].(map[string]any)
					if !ok {
						t.Errorf("document entry %d: expected 'extracted_fields' to be a map, got %v (type %T)", i, doc["extracted_fields"], doc["extracted_fields"])
					} else {
						if dt, ok := extractedFields["document_type"].(string); !ok || dt != tc.wantOrder[i].docType {
							t.Errorf("document entry %d: extracted_fields.document_type: want %q, got %q (type %T)", i, tc.wantOrder[i].docType, dt, extractedFields["document_type"])
						}
					}

					uploadedAtStr, ok := doc["source_uploaded_at"].(string)
					if !ok || uploadedAtStr == "" {
						t.Errorf("document entry %d: expected non-empty string 'source_uploaded_at' field, got %v (type %T)", i, doc["source_uploaded_at"], doc["source_uploaded_at"])
					} else {
						parsedTime, err := time.Parse(time.RFC3339, uploadedAtStr)
						if err != nil {
							t.Errorf("document entry %d: failed to parse source_uploaded_at %q: %v", i, uploadedAtStr, err)
						}
						uploadedAtTimes = append(uploadedAtTimes, parsedTime)
					}
				}

				if len(uploadedAtTimes) > 1 {
					if !(uploadedAtTimes[0].Before(uploadedAtTimes[1]) || uploadedAtTimes[0].Equal(uploadedAtTimes[1])) {
						t.Errorf("documents not in ingestion order: %v", uploadedAtTimes)
					}
				}

			} else { // 4xx
				wantErrorEnvelope(t, body)
			}
		})
	}
}
