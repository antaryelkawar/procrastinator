package documents

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"procrastinator-backend/internal/store"
	"procrastinator-backend/internal/store/assets"
	"procrastinator-backend/internal/store/sources"
)

// testSchema is a private schema owned by this test package. Each DB test
// package gets its own private schema because `go test` runs package test
// binaries concurrently (with default -p) and they share one database — a
// shared public schema would make per-test TRUNCATE isolation race across
// packages.
const testSchema = "lm_documents"

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

// schemaDSN returns the test DSN with search_path pinned to this package's
// private testSchema, so unqualified DDL/DML lands there.
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
	// Package dir is internal/store/documents, so the module-level migrations
	// directory is three levels up.
	return filepath.Join("..", "..", "..", "migrations")
}

func strPtr(s string) *string {
	return &s
}

func randUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("failed to read random bytes: %v", err)
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func seedAsset(t *testing.T, st *store.Store, serial string) assets.Asset {
	t.Helper()
	ctx := context.Background()
	a, err := assets.New(st.Pool()).Create(ctx, assets.Asset{SerialNumber: strPtr(serial)})
	if err != nil {
		t.Fatalf("seedAsset: %v", err)
	}
	return a
}

func seedSource(t *testing.T, st *store.Store, filename string) sources.Source {
	t.Helper()
	ctx := context.Background()
	s, err := sources.New(st.Pool()).Create(ctx, sources.Source{
		Filename:    filename,
		ContentType: "application/pdf",
		ByteSize:    4096,
		StoragePath: "storage/" + filename,
		SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	})
	if err != nil {
		t.Fatalf("seedSource: %v", err)
	}
	return s
}

// assertJSONBSemantic unmarshals both raw values into any and compares with
// reflect.DeepEqual so key-order differences do not cause false negatives.
func assertJSONBSemantic(t *testing.T, label string, got, want []byte) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("%s: unmarshal got: %v (raw=%q)", label, err, got)
	}
	if err := json.Unmarshal(want, &wantVal); err != nil {
		t.Fatalf("%s: unmarshal want: %v (raw=%q)", label, err, want)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("%s: got %s, want %s", label, got, want)
	}
}

// All tests run sequentially because they share the same database tables
// and truncate them for isolation. t.Parallel() is intentionally omitted.

func TestCreateLinksSourceAndAsset(t *testing.T) {
	cases := []struct {
		name            string
		docType         DocType
		extractedFields string
		rawExtraction   string
	}{
		{
			name:            "invoice links source and asset",
			docType:         TypeInvoice,
			extractedFields: `{"brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-123-ABC","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`,
			rawExtraction:   `{"document_type":"invoice","brand":"Samsung","model":"WW90T534DAW","serial_number":"SN-123-ABC","purchase_date":"2024-03-15","price":"1299.99","currency":"EUR","warranty_start":"2024-03-15","warranty_end":"2026-03-15"}`,
		},
		{
			name:            "warranty links source and asset",
			docType:         TypeWarranty,
			extractedFields: `{"brand":"Acme","model":"W-200","serial_number":"ACME-77","warranty_start":"2023-11-01","warranty_end":"2025-11-01"}`,
			rawExtraction:   `{"document_type":"warranty","brand":"Acme","model":"W-200","serial_number":"ACME-77","warranty_start":"2023-11-01","warranty_end":"2025-11-01"}`,
		},
		{
			name:            "other links source and asset",
			docType:         TypeOther,
			extractedFields: `{"note":"manual entry"}`,
			rawExtraction:   `{"document_type":"other","note":"manual entry"}`,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			asset := seedAsset(t, st, fmt.Sprintf("DOC-%d-%s", i, tc.name))
			src := seedSource(t, st, fmt.Sprintf("%s.pdf", tc.name))

			created, err := repo.Create(ctx, Document{
				AssetID:         asset.ID,
				SourceID:        src.ID,
				DocType:         tc.docType,
				ExtractedFields: json.RawMessage(tc.extractedFields),
				RawExtraction:   json.RawMessage(tc.rawExtraction),
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if created.ID == "" {
				t.Error("ID: got empty, want non-empty")
			}
			if created.AssetID != asset.ID {
				t.Errorf("AssetID: got %q, want %q", created.AssetID, asset.ID)
			}
			if created.SourceID != src.ID {
				t.Errorf("SourceID: got %q, want %q", created.SourceID, src.ID)
			}
			if created.DocType != tc.docType {
				t.Errorf("DocType: got %q, want %q", created.DocType, tc.docType)
			}
			now := time.Now()
			if created.CreatedAt.Before(now.Add(-time.Minute)) || created.CreatedAt.After(now.Add(time.Minute)) {
				t.Errorf("CreatedAt: %v is not within 1 minute of now", created.CreatedAt)
			}
			assertJSONBSemantic(t, "ExtractedFields (Create)", created.ExtractedFields, []byte(tc.extractedFields))
			assertJSONBSemantic(t, "RawExtraction (Create)", created.RawExtraction, []byte(tc.rawExtraction))

			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.ID != created.ID {
				t.Errorf("GetByID ID: got %q, want %q", got.ID, created.ID)
			}
			if got.AssetID != asset.ID {
				t.Errorf("GetByID AssetID: got %q, want %q", got.AssetID, asset.ID)
			}
			if got.SourceID != src.ID {
				t.Errorf("GetByID SourceID: got %q, want %q", got.SourceID, src.ID)
			}
			if got.DocType != tc.docType {
				t.Errorf("GetByID DocType: got %q, want %q", got.DocType, tc.docType)
			}
			assertJSONBSemantic(t, "ExtractedFields (GetByID)", got.ExtractedFields, []byte(tc.extractedFields))
			assertJSONBSemantic(t, "RawExtraction (GetByID)", got.RawExtraction, []byte(tc.rawExtraction))
		})
	}
}

func TestCreateDocTypeValidation(t *testing.T) {
	cases := []struct {
		name    string
		docType DocType
		wantErr bool
	}{
		{name: "invoice accepted", docType: TypeInvoice, wantErr: false},
		{name: "warranty accepted", docType: TypeWarranty, wantErr: false},
		{name: "other accepted", docType: TypeOther, wantErr: false},
		{name: "unknown type rejected", docType: DocType("receipt"), wantErr: true},
		{name: "empty type rejected", docType: DocType(""), wantErr: true},
		{name: "uppercase type rejected", docType: DocType("INVOICE"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			if !tc.wantErr {
				asset := seedAsset(t, st, "DT-"+tc.name)
				src := seedSource(t, st, tc.name+".pdf")

				created, err := repo.Create(ctx, Document{
					AssetID:         asset.ID,
					SourceID:        src.ID,
					DocType:         tc.docType,
					ExtractedFields: json.RawMessage(`{"note":"v"}`),
					RawExtraction:   json.RawMessage(`{"document_type":"` + string(tc.docType) + `"}`),
				})
				if err != nil {
					t.Fatalf("Create: %v", err)
				}
				if created.DocType != tc.docType {
					t.Errorf("DocType: got %q, want %q", created.DocType, tc.docType)
				}
			} else {
				_, err := repo.Create(ctx, Document{
					AssetID:         "00000000-0000-0000-0000-000000000000",
					SourceID:        "00000000-0000-0000-0000-000000000000",
					DocType:         tc.docType,
					ExtractedFields: json.RawMessage(`{}`),
					RawExtraction:   json.RawMessage(`{}`),
				})
				if !errors.Is(err, ErrInvalidDocType) {
					t.Fatalf("expected ErrInvalidDocType, got %v", err)
				}
			}
		})
	}
}

func TestCreateRejectsBadReferences(t *testing.T) {
	cases := []struct {
		name string
		fn   func(t *testing.T, st *store.Store)
	}{
		{
			name: "unknown asset id is rejected",
			fn: func(t *testing.T, st *store.Store) {
				ctx := context.Background()
				repo := New(st.Pool())
				src := seedSource(t, st, "ref-asset.pdf")

				_, err := repo.Create(ctx, Document{
					AssetID:         randUUID(t),
					SourceID:        src.ID,
					DocType:         TypeInvoice,
					ExtractedFields: json.RawMessage(`{}`),
					RawExtraction:   json.RawMessage(`{}`),
				})
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
		{
			name: "unknown source id is rejected",
			fn: func(t *testing.T, st *store.Store) {
				ctx := context.Background()
				repo := New(st.Pool())
				asset := seedAsset(t, st, "REF-SRC")

				_, err := repo.Create(ctx, Document{
					AssetID:         asset.ID,
					SourceID:        randUUID(t),
					DocType:         TypeInvoice,
					ExtractedFields: json.RawMessage(`{}`),
					RawExtraction:   json.RawMessage(`{}`),
				})
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			tc.fn(t, st)
		})
	}
}

func TestCreateDuplicateSourceRejected(t *testing.T) {
	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	asset := seedAsset(t, st, "DUP-1")
	src := seedSource(t, st, "dup.pdf")

	_, err := repo.Create(ctx, Document{
		AssetID:         asset.ID,
		SourceID:        src.ID,
		DocType:         TypeInvoice,
		ExtractedFields: json.RawMessage(`{"seq":1}`),
		RawExtraction:   json.RawMessage(`{"document_type":"invoice","seq":1}`),
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = repo.Create(ctx, Document{
		AssetID:         asset.ID,
		SourceID:        src.ID,
		DocType:         TypeWarranty,
		ExtractedFields: json.RawMessage(`{"seq":2}`),
		RawExtraction:   json.RawMessage(`{"document_type":"warranty","seq":2}`),
	})
	if !errors.Is(err, ErrSourceAlreadyLinked) {
		t.Fatalf("second Create: expected ErrSourceAlreadyLinked, got %v", err)
	}

	got, err := repo.ListByAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("ListByAsset len: got %d, want 1", len(got))
	}
}

func TestListByAssetIngestionOrder(t *testing.T) {
	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	asset := seedAsset(t, st, "ORDER-1")
	asset2 := seedAsset(t, st, "ORDER-2")

	filenames := []string{"first.pdf", "second.png", "third.jpg"}
	var srcs []sources.Source
	for _, fn := range filenames {
		srcs = append(srcs, seedSource(t, st, fn))
	}

	doc1, err := repo.Create(ctx, Document{
		AssetID:         asset.ID,
		SourceID:        srcs[0].ID,
		DocType:         TypeInvoice,
		ExtractedFields: json.RawMessage(`{"seq":1}`),
		RawExtraction:   json.RawMessage(`{"document_type":"invoice","seq":1}`),
	})
	if err != nil {
		t.Fatalf("Create doc1: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	doc2, err := repo.Create(ctx, Document{
		AssetID:         asset.ID,
		SourceID:        srcs[1].ID,
		DocType:         TypeWarranty,
		ExtractedFields: json.RawMessage(`{"seq":2}`),
		RawExtraction:   json.RawMessage(`{"document_type":"warranty","seq":2}`),
	})
	if err != nil {
		t.Fatalf("Create doc2: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	doc3, err := repo.Create(ctx, Document{
		AssetID:         asset.ID,
		SourceID:        srcs[2].ID,
		DocType:         TypeOther,
		ExtractedFields: json.RawMessage(`{"seq":3}`),
		RawExtraction:   json.RawMessage(`{"document_type":"other","seq":3}`),
	})
	if err != nil {
		t.Fatalf("Create doc3: %v", err)
	}

	src2 := seedSource(t, st, "other-asset.pdf")
	doc4, err := repo.Create(ctx, Document{
		AssetID:         asset2.ID,
		SourceID:        src2.ID,
		DocType:         TypeInvoice,
		ExtractedFields: json.RawMessage(`{"seq":4}`),
		RawExtraction:   json.RawMessage(`{"document_type":"invoice","seq":4}`),
	})
	if err != nil {
		t.Fatalf("Create doc4: %v", err)
	}

	docs := []Document{doc1, doc2, doc3}
	wantTypes := []DocType{TypeInvoice, TypeWarranty, TypeOther}

	got, err := repo.ListByAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListByAsset len: got %d, want 3", len(got))
	}
	for i := 0; i < 3; i++ {
		if got[i].ID != docs[i].ID {
			t.Errorf("position %d: ID got %q, want %q", i, got[i].ID, docs[i].ID)
		}
		if got[i].DocType != wantTypes[i] {
			t.Errorf("position %d: DocType got %q, want %q", i, got[i].DocType, wantTypes[i])
		}
		if got[i].SourceFilename != srcs[i].Filename {
			t.Errorf("position %d: SourceFilename got %q, want %q", i, got[i].SourceFilename, srcs[i].Filename)
		}
		if !got[i].SourceUploadedAt.Equal(srcs[i].UploadedAt) {
			t.Errorf("position %d: SourceUploadedAt got %v, want %v", i, got[i].SourceUploadedAt, srcs[i].UploadedAt)
		}
		if got[i].AssetID != asset.ID {
			t.Errorf("position %d: AssetID got %q, want %q", i, got[i].AssetID, asset.ID)
		}
	}

	got2, err := repo.ListByAsset(ctx, asset2.ID)
	if err != nil {
		t.Fatalf("ListByAsset (asset2): %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("ListByAsset (asset2) len: got %d, want 1", len(got2))
	}
	if got2[0].ID != doc4.ID {
		t.Errorf("asset2 position 0: ID got %q, want %q", got2[0].ID, doc4.ID)
	}
}

func TestListByAssetUnknownAsset(t *testing.T) {
	t.Parallel()

	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	got, err := repo.ListByAsset(ctx, randUUID(t))
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len: got %d, want 0", len(got))
	}
	if got == nil {
		t.Error("got is nil, want non-nil empty slice")
	}
}

func TestGetByIDUnknown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
	}{
		{
			name: "random uuid yields ErrNotFound",
			id:   "", // placeholder — set from randUUID in test body
		},
		{
			name: "malformed id yields an error",
			id:   "not-a-uuid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			id := tc.id
			if id == "" {
				id = randUUID(t)
			}

			_, err := repo.GetByID(ctx, id)
			if tc.name == "random uuid yields ErrNotFound" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			}
		})
	}
}
