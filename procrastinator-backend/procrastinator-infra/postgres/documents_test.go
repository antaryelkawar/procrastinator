package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-infra/postgres"
)

const testSchemaDocuments = "p_documents"

var (
	docOnce       sync.Once
	docPool       *pgxpool.Pool
	docInitErr    error
	docRepo       data.DocumentRepository
	docAssetRepo  data.AssetRepository
	docSourceRepo data.SourceRepository
)

type docsTestRepos struct {
	Documents data.DocumentRepository
	Assets    data.AssetRepository
	Sources   data.SourceRepository
}

func docsRepos(t *testing.T) docsTestRepos {
	t.Helper()
	docOnce.Do(func() {
		docPool, docInitErr = openTestPool(t, testSchemaDocuments)
		if docInitErr != nil {
			return
		}
		docRepo = postgres.NewDocumentRepo(docPool)
		docAssetRepo = postgres.NewAssetRepo(docPool)
		docSourceRepo = postgres.NewSourceRepo(docPool)
	})
	if docInitErr != nil {
		t.Fatalf("init documents test pool: %v", docInitErr)
	}
	return docsTestRepos{
		Documents: docRepo,
		Assets:    docAssetRepo,
		Sources:   docSourceRepo,
	}
}

// truncateDocs clears all test data. Call at the top of each write test.
func truncateDocs(t *testing.T) {
	t.Helper()
	if _, err := docPool.Exec(context.Background(), `TRUNCATE documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate documents: %v", err)
	}
}

// setupDocs returns repositories with all tables cleared.
func setupDocs(t *testing.T) docsTestRepos {
	t.Helper()
	repos := docsRepos(t)
	truncateDocs(t)
	return repos
}

func seedAsset(t *testing.T, repos docsTestRepos, tenant string) data.Asset {
	t.Helper()
	a, err := repos.Assets.Create(ctxWithTenant(tenant), testAsset())
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	return a
}

func seedSource(t *testing.T, repos docsTestRepos, tenant string) data.Source {
	t.Helper()
	s, err := repos.Sources.Create(ctxWithTenant(tenant), testSource())
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	return s
}

func assertJSONMapEqual(t *testing.T, name string, got, want map[string]any) {
	t.Helper()
	// encoding/json sorts map keys, so marshaled bytes are comparable.
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: marshal got: %v", name, err)
	}
	wantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("%s: marshal want: %v", name, err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Errorf("%s: got %s, want %s", name, gotBytes, wantBytes)
	}
}

func TestDocumentCreate(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	asset := seedAsset(t, repos, tenantA)
	src := seedSource(t, repos, tenantA)

	raw := `<thought>classification: amc, serial WM-2024-001</thought>{"classification":"amc","serial_number":"WM-2024-001","price":"39999.99"}`
	fields := map[string]any{
		"serial_number": "WM-2024-001",
		"price":         "39999.99",
		"metadata": map[string]any{
			"invoice_no":      "INV-42",
			"amc_card_number": "AC-1001",
		},
	}

	created, err := repos.Documents.Create(ctxA, data.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         data.DocTypeAMC,
		ExtractedFields: fields,
		RawExtraction:   raw,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if created.TenantID != tenantA {
		t.Errorf("Create TenantID = %q, want %q", created.TenantID, tenantA)
	}
	if created.CreatedAt.IsZero() {
		t.Error("Create CreatedAt is zero, want DB default now()")
	}

	got, err := repos.Documents.GetByID(ctxA, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SourceID != created.SourceID {
		t.Errorf("SourceID = %q, want %q", got.SourceID, created.SourceID)
	}
	if got.AssetID != created.AssetID {
		t.Errorf("AssetID = %q, want %q", got.AssetID, created.AssetID)
	}
	if got.DocType != created.DocType {
		t.Errorf("DocType = %q, want %q", got.DocType, created.DocType)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created.CreatedAt)
	}
	assertJSONMapEqual(t, "ExtractedFields", got.ExtractedFields, fields)
	if got.RawExtraction != raw {
		t.Errorf("RawExtraction = %q, want verbatim %q", got.RawExtraction, raw)
	}
}

func TestDocumentCreateFKEnforced(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	asset := seedAsset(t, repos, tenantA)
	src := seedSource(t, repos, tenantA)

	zeroID := "00000000-0000-0000-0000-000000000000"

	cases := []struct {
		name     string
		sourceID string
		assetID  string
	}{
		{"unknown asset", src.ID, zeroID},
		{"unknown source", zeroID, asset.ID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := repos.Documents.Create(ctxA, data.Document{
				SourceID:        tc.sourceID,
				AssetID:         tc.assetID,
				DocType:         data.DocTypeInvoice,
				ExtractedFields: map[string]any{},
			})
			if err == nil {
				t.Errorf("Create(%s): err = nil, want FK violation error", tc.name)
			}
		})
	}
}

func TestDocumentSourceLinkUniqueness(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	asset := seedAsset(t, repos, tenantA)
	src := seedSource(t, repos, tenantA)

	fields := map[string]any{}

	if _, err := repos.Documents.Create(ctxA, data.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         data.DocTypeInvoice,
		ExtractedFields: fields,
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if _, err := repos.Documents.Create(ctxA, data.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         data.DocTypeWarranty,
		ExtractedFields: fields,
	}); !errors.Is(err, postgres.ErrSourceAlreadyLinked) {
		t.Errorf("second Create(same source): err = %v, want ErrSourceAlreadyLinked", err)
	}

	// A second document may link the same asset with a different source.
	src2 := seedSource(t, repos, tenantA)
	if _, err := repos.Documents.Create(ctxA, data.Document{
		SourceID:        src2.ID,
		AssetID:         asset.ID,
		DocType:         data.DocTypeAMC,
		ExtractedFields: fields,
	}); err != nil {
		t.Errorf("third Create(second source, same asset): %v, want success", err)
	}
}

func TestDocumentDocTypeValidation(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	asset := seedAsset(t, repos, tenantA)

	cases := []struct {
		name    string
		docType string
		wantErr bool
	}{
		{"invoice", data.DocTypeInvoice, false},
		{"warranty", data.DocTypeWarranty, false},
		{"amc", data.DocTypeAMC, false},
		{"other", data.DocTypeOther, false},
		{"uppercase rejected", "INVOICE", true},
		{"trailing space rejected", "invoice ", true},
		{"unknown rejected", "receipt", true},
		{"empty rejected", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Each case seeds its own source so valid cases don't trip the
			// source-uniqueness constraint.
			src := seedSource(t, repos, tenantA)
			_, err := repos.Documents.Create(ctxA, data.Document{
				SourceID:        src.ID,
				AssetID:         asset.ID,
				DocType:         tc.docType,
				ExtractedFields: map[string]any{},
			})
			if tc.wantErr && err == nil {
				t.Errorf("Create(doc_type %q): err = nil, want error", tc.docType)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Create(doc_type %q): unexpected err %v", tc.docType, err)
			}
		})
	}

	// Only the four valid doc_types inserted anything.
	list, err := repos.Documents.ListByAsset(ctxA, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(list) != 4 {
		t.Errorf("ListByAsset = %d rows, want exactly 4 (valid doc_types only)", len(list))
	}
}

func TestDocumentListByAsset(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	asset := seedAsset(t, repos, tenantA)

	filenames := []string{"invoice.pdf", "warranty.pdf", "amc.pdf"}
	sizes := []int64{100, 200, 300}
	sha256s := []string{
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"027d95674e9d4a4bce1c2c5c8e1c2c5c8e1c2c5c8e1c2c5c8e1c2c5c8e1c2c5c",
	}
	uploadedAts := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC),
	}
	docTypes := []string{data.DocTypeInvoice, data.DocTypeWarranty, data.DocTypeAMC}

	createdIDs := make([]string, 0, 3)
	for i := range filenames {
		src, err := repos.Sources.Create(ctxA, data.Source{
			Filename:    filenames[i],
			ContentType: "application/pdf",
			Size:        sizes[i],
			Path:        "storage/" + filenames[i],
			SHA256:      sha256s[i],
			UploadedAt:  uploadedAts[i],
		})
		if err != nil {
			t.Fatalf("Create source[%d]: %v", i, err)
		}
		created, err := repos.Documents.Create(ctxA, data.Document{
			SourceID:        src.ID,
			AssetID:         asset.ID,
			DocType:         docTypes[i],
			ExtractedFields: map[string]any{},
		})
		if err != nil {
			t.Fatalf("Create document[%d]: %v", i, err)
		}
		createdIDs = append(createdIDs, created.ID)
		if i < len(filenames)-1 {
			// guarantee distinct created_at for the ordering assertion
			time.Sleep(time.Millisecond)
		}
	}

	list, err := repos.Documents.ListByAsset(ctxA, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListByAsset = %d rows, want 3", len(list))
	}
	for i, got := range list {
		if got.ID != createdIDs[i] {
			t.Errorf("order at %d: ID = %q, want %q", i, got.ID, createdIDs[i])
		}
		if got.SourceFilename != filenames[i] {
			t.Errorf("row %d: SourceFilename = %q, want %q", i, got.SourceFilename, filenames[i])
		}
		if !got.SourceUploadedAt.Equal(uploadedAts[i]) {
			t.Errorf("row %d: SourceUploadedAt = %v, want %v", i, got.SourceUploadedAt, uploadedAts[i])
		}
	}
}

func TestDocumentListByAssetUnknownAsset(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	zeroID := "00000000-0000-0000-0000-000000000000"
	list, err := repos.Documents.ListByAsset(ctxA, zeroID)
	if err != nil {
		t.Fatalf("ListByAsset(unknown asset): %v, want nil error", err)
	}
	if list == nil {
		t.Error("ListByAsset(unknown asset) = nil, want non-nil empty slice")
	}
	if len(list) != 0 {
		t.Errorf("ListByAsset(unknown asset) = %d rows, want 0", len(list))
	}
}

func TestDocumentTenantIsolation(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)
	ctxB := ctxWithTenant(tenantB)

	asset := seedAsset(t, repos, tenantA)
	src := seedSource(t, repos, tenantA)

	doc, err := repos.Documents.Create(ctxA, data.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         data.DocTypeInvoice,
		ExtractedFields: map[string]any{},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// GetByID under the other tenant must not see the document.
	if _, err := repos.Documents.GetByID(ctxB, doc.ID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(other tenant): err = %v, want ErrNotFound", err)
	}

	// ListByAsset under the other tenant with the foreign asset must be empty.
	list, err := repos.Documents.ListByAsset(ctxB, asset.ID)
	if err != nil {
		t.Fatalf("ListByAsset(other tenant): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListByAsset(other tenant) = %d rows, want 0", len(list))
	}
}

func TestDocumentGetByIDNotFound(t *testing.T) {
	repos := setupDocs(t)
	ctxA := ctxWithTenant(tenantA)

	zeroID := "00000000-0000-0000-0000-000000000000"
	if _, err := repos.Documents.GetByID(ctxA, zeroID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(random): err = %v, want ErrNotFound", err)
	}
}

func TestDocumentTenantLessContext(t *testing.T) {
	repos := setupDocs(t)
	bg := context.Background()

	asset := seedAsset(t, repos, tenantA)
	src := seedSource(t, repos, tenantA)

	zeroID := "00000000-0000-0000-0000-000000000000"

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			_, err := repos.Documents.Create(bg, data.Document{
				SourceID:        src.ID,
				AssetID:         asset.ID,
				DocType:         data.DocTypeInvoice,
				ExtractedFields: map[string]any{},
			})
			return err
		}},
		{"GetByID", func() error {
			_, err := repos.Documents.GetByID(bg, zeroID)
			return err
		}},
		{"ListByAsset", func() error {
			_, err := repos.Documents.ListByAsset(bg, asset.ID)
			return err
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, data.ErrNoTenant) {
				t.Errorf("%s: err = %v, want ErrNoTenant", tc.name, err)
			}
		})
	}
}
