package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/identity"
	"procrastinator-backend/internal/store"
	"procrastinator-backend/internal/store/documents"
	"procrastinator-backend/internal/store/sources"
)

const testSchema = "lm_ingest"

var testDSN string

func TestMain(m *testing.M) {
	testDSN = os.Getenv("LM_TEST_DATABASE_URL")
	if testDSN != "" {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, testDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap pool: %v\n", err)
			os.Exit(1)
		}
		if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE"); err != nil {
			pool.Close()
			fmt.Fprintf(os.Stderr, "drop schema: %v\n", err)
			os.Exit(1)
		}
		if _, err := pool.Exec(ctx, "CREATE SCHEMA "+testSchema); err != nil {
			pool.Close()
			fmt.Fprintf(os.Stderr, "create schema: %v\n", err)
			os.Exit(1)
		}
		pool.Close()
	}
	os.Exit(m.Run())
}

func schemaDSN() string {
	u, err := url.Parse(testDSN)
	if err != nil {
		return testDSN
	}
	q := u.Query()
	q.Set("search_path", testSchema)
	u.RawQuery = q.Encode()
	return u.String()
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	if testDSN == "" {
		t.Skip("LM_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	st, err := store.Open(context.Background(), schemaDSN())
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	if err := st.Migrate(ctx, migrationsDir()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	if err := st.TruncateAll(ctx); err != nil {
		t.Fatalf("failed to truncate after migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := st.TruncateAll(context.Background()); err != nil {
			t.Logf("cleanup truncate failed: %v", err)
		}
	})
	return st
}

func migrationsDir() string {
	// Package dir is internal/ingest, so the module-level migrations
	// directory is two levels up.
	return filepath.Join("..", "..", "migrations")
}

func countRows(t *testing.T, st *store.Store, table string) int64 {
	t.Helper()
	var count int64
	err := st.Pool().QueryRow(context.Background(), fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count)
	if err != nil {
		t.Fatalf("countRows %s: %v", table, err)
	}
	return count
}

// fakeExtractor implements the Extractor interface for testing.
// Tests are sequential (no t.Parallel), so no mutex is needed.
type fakeExtractor struct {
	raw             []byte
	err             error
	calls           int
	lastContentType string
	lastData        []byte
}

func (f *fakeExtractor) Extract(_ context.Context, contentType string, data []byte) ([]byte, error) {
	f.calls++
	f.lastContentType = contentType
	f.lastData = make([]byte, len(data))
	copy(f.lastData, data)
	return f.raw, f.err
}

// Raw payload constants for tests
var (
	invoiceRaw   = []byte(`{"document_type":"invoice","brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-200","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`)
	warrantyRaw2 = []byte(`{"document_type":"warranty","serial_number":"sn-200","warranty_start":"2024-03-15","warranty_end":"2028-12-31"}`)
)

func newTestService(t *testing.T, st *store.Store, fx Extractor) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{StorageDir: dir, MaxUploadBytes: 1 << 20}
	return NewService(cfg, st.Pool(), fx), dir
}

// All tests run sequentially because they share the same database tables
// and truncate them for isolation. t.Parallel() is intentionally omitted.

func TestProcessPersistsSource(t *testing.T) {
	st := openTestStore(t)
	fx := &fakeExtractor{raw: invoiceRaw}
	svc, _ := newTestService(t, st, fx)
	ctx := context.Background()

	asset, err := svc.Process(ctx, "invoice.pdf", pdfFixture)
	if err != nil {
		t.Fatalf("Process returned unexpected error: %v", err)
	}
	if asset.ID == "" {
		t.Fatal("asset.ID is empty")
	}

	// Check asset fields
	if asset.Brand == nil || *asset.Brand != "Samsung" {
		t.Errorf("Brand: got %v, want Samsung", asset.Brand)
	}
	if asset.Model == nil || *asset.Model != "WW90T534DAW" {
		t.Errorf("Model: got %v, want WW90T534DAW", asset.Model)
	}
	if asset.SerialNumber == nil || *asset.SerialNumber != "SN-200" {
		t.Errorf("SerialNumber: got %v, want SN-200", asset.SerialNumber)
	}
	if asset.Price == nil || *asset.Price != "1299.99" {
		t.Errorf("Price: got %v, want 1299.99", asset.Price)
	}
	if asset.Currency == nil || *asset.Currency != "EUR" {
		t.Errorf("Currency: got %v, want EUR", asset.Currency)
	}
	if asset.PurchaseDate == nil || asset.PurchaseDate.Format("2006-01-02") != "2024-03-15" {
		t.Errorf("PurchaseDate: got %v, want 2024-03-15", asset.PurchaseDate)
	}
	if asset.WarrantyEnd == nil || asset.WarrantyEnd.Format("2006-01-02") != "2026-03-15" {
		t.Errorf("WarrantyEnd: got %v, want 2026-03-15", asset.WarrantyEnd)
	}

	// Check document and source
	docsRepo := documents.New(st.Pool())
	docs, err := docsRepo.ListByAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("ListByAsset len: got %d, want 1", len(docs))
	}

	srcRepo := sources.New(st.Pool())
	src, err := srcRepo.GetByID(ctx, docs[0].SourceID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if src.Filename != "invoice.pdf" {
		t.Errorf("Filename: got %q, want invoice.pdf", src.Filename)
	}
	if src.ContentType != "application/pdf" {
		t.Errorf("ContentType: got %q, want application/pdf", src.ContentType)
	}
	if src.ByteSize != int64(len(pdfFixture)) {
		t.Errorf("ByteSize: got %d, want %d", src.ByteSize, len(pdfFixture))
	}

	hash := sha256.Sum256(pdfFixture)
	wantHash := hex.EncodeToString(hash[:])
	if src.SHA256 != wantHash {
		t.Errorf("SHA256: got %q, want %q", src.SHA256, wantHash)
	}
	if src.StoragePath == "" {
		t.Error("StoragePath is empty")
	}

	// Check file exists and has correct bytes
	got, err := os.ReadFile(src.StoragePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, pdfFixture) {
		t.Error("file contents differ from input")
	}

	// Check UploadedAt within 1 minute of now
	now := time.Now()
	if src.UploadedAt.Before(now.Add(-time.Minute)) || src.UploadedAt.After(now.Add(time.Minute)) {
		t.Errorf("UploadedAt: %v is not within 1 minute of now", src.UploadedAt)
	}

	// Check extractor was called correctly
	if fx.calls != 1 {
		t.Errorf("fx.calls: got %d, want 1", fx.calls)
	}
	if fx.lastContentType != "application/pdf" {
		t.Errorf("fx.lastContentType: got %q, want application/pdf", fx.lastContentType)
	}
	if !bytes.Equal(fx.lastData, pdfFixture) {
		t.Error("fx.lastData differs from pdfFixture")
	}
}

func TestProcessExtractionFailureKeepsSource(t *testing.T) {
	cases := []struct {
		name  string
		setup func(fx *fakeExtractor)
	}{
		{
			name: "extractor returns error",
			setup: func(fx *fakeExtractor) {
				fx.err = errors.New("connection refused")
			},
		},
		{
			name: "unparseable extraction payload",
			setup: func(fx *fakeExtractor) {
				fx.raw = []byte("<thought>thinking out loud</thought>not json at all")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			fx := &fakeExtractor{}
			tc.setup(fx)
			svc, _ := newTestService(t, st, fx)
			ctx := context.Background()

			_, err := svc.Process(ctx, "fail.pdf", pdfFixture)
			if !errors.Is(err, ErrExtraction) {
				t.Fatalf("expected ErrExtraction, got %v", err)
			}

			// Check source retained
			if countRows(t, st, "sources") != 1 {
				t.Errorf("sources count: got %d, want 1", countRows(t, st, "sources"))
			}
			if countRows(t, st, "assets") != 0 {
				t.Errorf("assets count: got %d, want 0", countRows(t, st, "assets"))
			}
			if countRows(t, st, "documents") != 0 {
				t.Errorf("documents count: got %d, want 0", countRows(t, st, "documents"))
			}

			// Check source metadata
			var filename, contentType, storagePath, sha256 string
			var byteSize int64
			err = st.Pool().QueryRow(ctx, "SELECT filename, content_type, byte_size, sha256, storage_path FROM sources").Scan(&filename, &contentType, &byteSize, &sha256, &storagePath)
			if err != nil {
				t.Fatalf("query sources: %v", err)
			}
			if filename != "fail.pdf" {
				t.Errorf("filename: got %q, want fail.pdf", filename)
			}
			if contentType != "application/pdf" {
				t.Errorf("content_type: got %q, want application/pdf", contentType)
			}
			if byteSize != int64(len(pdfFixture)) {
				t.Errorf("byte_size: got %d, want %d", byteSize, len(pdfFixture))
			}

			// Check file bytes intact
			got, err := os.ReadFile(storagePath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, pdfFixture) {
				t.Error("file contents differ from input")
			}
		})
	}
}

func TestProcessNoIdentityKeepsSource(t *testing.T) {
	st := openTestStore(t)
	fx := &fakeExtractor{raw: []byte(`{"document_type":"other","note":"no identity fields"}`)}
	svc, _ := newTestService(t, st, fx)
	ctx := context.Background()

	_, err := svc.Process(ctx, "noid.pdf", pdfFixture)
	if !errors.Is(err, identity.ErrNoIdentity) {
		t.Fatalf("expected identity.ErrNoIdentity, got %v", err)
	}

	if countRows(t, st, "sources") != 1 {
		t.Errorf("sources count: got %d, want 1", countRows(t, st, "sources"))
	}
	if countRows(t, st, "assets") != 0 {
		t.Errorf("assets count: got %d, want 0", countRows(t, st, "assets"))
	}
	if countRows(t, st, "documents") != 0 {
		t.Errorf("documents count: got %d, want 0", countRows(t, st, "documents"))
	}
}

func TestProcessCreatesDocumentWithRawPayload(t *testing.T) {
	st := openTestStore(t)
	raw := `<thought>This is an invoice for a washing machine.</thought>` + string(invoiceRaw)
	fx := &fakeExtractor{raw: []byte(raw)}
	svc, _ := newTestService(t, st, fx)
	ctx := context.Background()

	asset, err := svc.Process(ctx, "thought.pdf", pdfFixture)
	if err != nil {
		t.Fatalf("Process returned unexpected error: %v", err)
	}

	docsRepo := documents.New(st.Pool())
	docs, err := docsRepo.ListByAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("ListByAsset len: got %d, want 1", len(docs))
	}

	if docs[0].DocType != documents.TypeInvoice {
		t.Errorf("DocType: got %q, want invoice", docs[0].DocType)
	}

	// Check RawExtraction
	var rawStr string
	if err := json.Unmarshal(docs[0].RawExtraction, &rawStr); err != nil {
		t.Fatalf("unmarshal RawExtraction: %v", err)
	}
	if rawStr != raw {
		t.Errorf("RawExtraction: got %q, want %q", rawStr, raw)
	}

	// Check ExtractedFields
	var ef map[string]any
	if err := json.Unmarshal(docs[0].ExtractedFields, &ef); err != nil {
		t.Fatalf("unmarshal ExtractedFields: %v", err)
	}

	checks := map[string]string{
		"document_type": "invoice",
		"brand":         "Samsung",
		"model":         "WW90T534DAW",
		"serial_number": "SN-200",
		"price":         "1299.99",
		"currency":      "EUR",
		"purchase_date": "2024-03-15",
		"warranty_end":  "2026-03-15",
	}
	for k, v := range checks {
		got, ok := ef[k]
		if !ok {
			t.Errorf("ExtractedFields missing key %q", k)
			continue
		}
		if got != v {
			t.Errorf("ExtractedFields[%q]: got %v, want %v", k, got, v)
		}
	}
}

func TestProcessSecondUploadSameSerialMerges(t *testing.T) {
	st := openTestStore(t)
	fx := &fakeExtractor{}
	svc, _ := newTestService(t, st, fx)
	ctx := context.Background()

	// First upload
	fx.raw = invoiceRaw
	asset1, err := svc.Process(ctx, "invoice.pdf", pdfFixture)
	if err != nil {
		t.Fatalf("first Process: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// Second upload
	fx.raw = warrantyRaw2
	asset2, err := svc.Process(ctx, "warranty.png", pngFixture)
	if err != nil {
		t.Fatalf("second Process: %v", err)
	}

	if asset2.ID != asset1.ID {
		t.Errorf("asset IDs differ: %q vs %q", asset2.ID, asset1.ID)
	}
	if countRows(t, st, "assets") != 1 {
		t.Errorf("assets count: got %d, want 1", countRows(t, st, "assets"))
	}
	if countRows(t, st, "sources") != 2 {
		t.Errorf("sources count: got %d, want 2", countRows(t, st, "sources"))
	}

	docsRepo := documents.New(st.Pool())
	docs, err := docsRepo.ListByAsset(ctx, asset1.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("ListByAsset len: got %d, want 2", len(docs))
	}

	if docs[0].DocType != documents.TypeInvoice {
		t.Errorf("docs[0].DocType: got %q, want invoice", docs[0].DocType)
	}
	if docs[1].DocType != documents.TypeWarranty {
		t.Errorf("docs[1].DocType: got %q, want warranty", docs[1].DocType)
	}
	if docs[0].SourceID == docs[1].SourceID {
		t.Error("docs[0].SourceID == docs[1].SourceID")
	}

	srcRepo := sources.New(st.Pool())
	src0, err := srcRepo.GetByID(ctx, docs[0].SourceID)
	if err != nil {
		t.Fatalf("GetByID docs[0]: %v", err)
	}
	if src0.Filename != "invoice.pdf" {
		t.Errorf("src0.Filename: got %q, want invoice.pdf", src0.Filename)
	}
	src1, err := srcRepo.GetByID(ctx, docs[1].SourceID)
	if err != nil {
		t.Fatalf("GetByID docs[1]: %v", err)
	}
	if src1.Filename != "warranty.png" {
		t.Errorf("src1.Filename: got %q, want warranty.png", src1.Filename)
	}

	// Merge assertions
	if asset2.Price == nil || *asset2.Price != "1299.99" {
		t.Errorf("Price: got %v, want 1299.99", asset2.Price)
	}
	if asset2.Currency == nil || *asset2.Currency != "EUR" {
		t.Errorf("Currency: got %v, want EUR", asset2.Currency)
	}
	if asset2.Brand == nil || *asset2.Brand != "Samsung" {
		t.Errorf("Brand: got %v, want Samsung", asset2.Brand)
	}
	if asset2.Model == nil || *asset2.Model != "WW90T534DAW" {
		t.Errorf("Model: got %v, want WW90T534DAW", asset2.Model)
	}
	if asset2.WarrantyEnd == nil || asset2.WarrantyEnd.Format("2006-01-02") != "2028-12-31" {
		t.Errorf("WarrantyEnd: got %v, want 2028-12-31", asset2.WarrantyEnd)
	}
	if asset2.SerialNumber == nil || *asset2.SerialNumber != "sn-200" {
		t.Errorf("SerialNumber: got %v, want sn-200", asset2.SerialNumber)
	}
}

func TestProcessRejectsUnsupportedType(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		filename string
	}{
		{name: "text", data: textFixture, filename: "notes.txt"},
		{name: "xml", data: xmlFixture, filename: "doc.xml"},
		{name: "gif", data: gifFixture, filename: "anim.gif"},
		{name: "empty", data: emptyFixture, filename: "empty.dat"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			fx := &fakeExtractor{}
			svc, dir := newTestService(t, st, fx)
			ctx := context.Background()

			_, err := svc.Process(ctx, tc.filename, tc.data)
			if !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("expected ErrUnsupportedType, got %v", err)
			}

			if countRows(t, st, "sources") != 0 {
				t.Errorf("sources count: got %d, want 0", countRows(t, st, "sources"))
			}
			if countRows(t, st, "assets") != 0 {
				t.Errorf("assets count: got %d, want 0", countRows(t, st, "assets"))
			}
			if countRows(t, st, "documents") != 0 {
				t.Errorf("documents count: got %d, want 0", countRows(t, st, "documents"))
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("storage dir not empty: got %d entries", len(entries))
			}
		})
	}
}

func TestProcessRejectsOversize(t *testing.T) {
	cases := []struct {
		name     string
		maxBytes int64
		data     []byte
		wantErr  bool
	}{
		{
			name:     "exceeds limit",
			maxBytes: 10,
			data:     []byte("%PDF-1.4\nxy"),
			wantErr:  true,
		},
		{
			name:     "exactly at limit accepted",
			maxBytes: 9,
			data:     []byte("%PDF-1.4\n"),
			wantErr:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			fx := &fakeExtractor{raw: invoiceRaw}
			cfg := config.Config{StorageDir: t.TempDir(), MaxUploadBytes: tc.maxBytes}
			svc := NewService(cfg, st.Pool(), fx)
			ctx := context.Background()

			_, err := svc.Process(ctx, "test.pdf", tc.data)
			if tc.wantErr {
				if !errors.Is(err, ErrTooLarge) {
					t.Fatalf("expected ErrTooLarge, got %v", err)
				}
				if countRows(t, st, "sources") != 0 {
					t.Errorf("sources count: got %d, want 0", countRows(t, st, "sources"))
				}
				entries, err := os.ReadDir(cfg.StorageDir)
				if err != nil {
					t.Fatalf("ReadDir: %v", err)
				}
				if len(entries) != 0 {
					t.Errorf("storage dir not empty: got %d entries", len(entries))
				}
			} else {
				if err != nil {
					t.Fatalf("Process returned unexpected error: %v", err)
				}
				if countRows(t, st, "sources") != 1 {
					t.Errorf("sources count: got %d, want 1", countRows(t, st, "sources"))
				}
			}
		})
	}
}
