package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-infra/postgres"
)

const (
	tenantA = "test-tenant"
	tenantB = "test-tenant-b"

	migrationsDir = "../../migrations"
	testSchema    = "p_assets"
)

var (
	repo data.AssetRepository
	dsn  string
	pool *pgxpool.Pool
)

func TestMain(m *testing.M) {
	dsn = os.Getenv("PROCRASTINATOR_TEST_DATABASE_URL")
	if dsn == "" {
		// No test database configured: skip all tests.
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse config:", err)
		os.Exit(1)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = testSchema + ",public"

	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pool:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		pool.Close()
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+testSchema); err != nil {
		fmt.Fprintln(os.Stderr, "create schema:", err)
		pool.Close()
		os.Exit(1)
	}
	// Run migrations using the pool with search_path set, so tables land in p_assets.
	stdDB := stdlib.OpenDBFromPool(pool)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintln(os.Stderr, "set dialect:", err)
		pool.Close()
		os.Exit(1)
	}
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		pool.Close()
		os.Exit(1)
	}

	repo = postgres.NewAssetRepo(pool)
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// truncate clears all test data. Call at the top of each write test.
func truncate(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func ctxWithTenant(tenant string) context.Context {
	return data.WithTenant(context.Background(), tenant)
}

func strPtr(s string) *string { return &s }

func testAsset(overrides ...func(*data.Asset)) data.Asset {
	a := data.Asset{
		TenantID:     tenantA,
		Brand:        strPtr("Samsung"),
		Model:        strPtr("WF80A"),
		SerialNumber: strPtr("WM-2024-001"),
		DocType:      data.DocTypeInvoice,
	}
	for _, fn := range overrides {
		fn(&a)
	}
	return a
}

func assertAssetEqual(t *testing.T, name string, got, want data.Asset) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s: ID = %q, want %q", name, got.ID, want.ID)
	}
	if got.TenantID != want.TenantID {
		t.Errorf("%s: TenantID = %q, want %q", name, got.TenantID, want.TenantID)
	}
	assertPtrEqual(t, name+".Brand", got.Brand, want.Brand)
	assertPtrEqual(t, name+".Model", got.Model, want.Model)
	assertPtrEqual(t, name+".SerialNumber", got.SerialNumber, want.SerialNumber)
	assertPtrEqual(t, name+".NormSerial", got.NormSerial, want.NormSerial)
	assertPtrEqual(t, name+".NormBrand", got.NormBrand, want.NormBrand)
	assertPtrEqual(t, name+".NormModel", got.NormModel, want.NormModel)
	assertTimePtrEqual(t, name+".PurchaseDate", got.PurchaseDate, want.PurchaseDate)
	assertTimePtrEqual(t, name+".WarrantyEnd", got.WarrantyEnd, want.WarrantyEnd)
	assertPtrEqual(t, name+".Price", got.Price, want.Price)
	assertPtrEqual(t, name+".Currency", got.Currency, want.Currency)
	if got.DocType != want.DocType {
		t.Errorf("%s: DocType = %q, want %q", name, got.DocType, want.DocType)
	}
	assertMetadataEqual(t, name+".Metadata", got.Metadata, want.Metadata)
}

func assertPtrEqual(t *testing.T, name string, got, want *string) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s: is nil, want %q", name, *want)
	case want == nil:
		t.Errorf("%s: = %q, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s: = %q, want %q", name, *got, *want)
	}
}

func assertTimePtrEqual(t *testing.T, name string, got, want *time.Time) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s: is nil, want %v", name, want)
	case want == nil:
		t.Errorf("%s: = %v, want nil", name, got)
	case !got.Equal(*want):
		t.Errorf("%s: = %v, want %v", name, got, want)
	}
}

func assertMetadataEqual(t *testing.T, name string, got, want map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len = %d, want %d (got %v, want %v)", name, len(got), len(want), got, want)
	}
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("%s: missing key %q", name, k)
			continue
		}
		if g != v {
			t.Errorf("%s: key %q = %v, want %v", name, k, g, v)
		}
	}
}

func TestCreateGetByID(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	purchase := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	warranty := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	created, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
		a.PurchaseDate = &purchase
		a.WarrantyEnd = &warranty
		a.Price = strPtr("39999.99")
		a.Currency = strPtr("INR")
		a.Metadata = map[string]any{"invoice_no": "INV-42", "vendor": "RetailMart"}
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	assertAssetEqual(t, "asset", got, created)
	if got.Metadata["invoice_no"] != "INV-42" {
		t.Errorf("Metadata[invoice_no] = %v, want INV-42", got.Metadata["invoice_no"])
	}
}

func TestTenantIsolation(t *testing.T) {
	truncate(t)

	created, err := repo.Create(ctxWithTenant(tenantA), testAsset())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// GetByID under the other tenant must not see the row.
	if _, err := repo.GetByID(ctxWithTenant(tenantB), created.ID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(other tenant): err = %v, want ErrNotFound", err)
	}

	// List under the other tenant must be empty.
	list, err := repo.List(ctxWithTenant(tenantB))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List(other tenant) = %d rows, want 0", len(list))
	}

	// FindBySerial / FindByBrandModel under the other tenant.
	if _, err := repo.FindBySerial(ctxWithTenant(tenantB), data.NormalizeSerial("WM-2024-001")); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("FindBySerial(other tenant): err = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByBrandModel(ctxWithTenant(tenantB), data.NormalizeName("Samsung"), data.NormalizeName("WF80A")); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("FindByBrandModel(other tenant): err = %v, want ErrNotFound", err)
	}
}

func TestCreatePartialAsset(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	created, err := repo.Create(ctx, data.Asset{DocType: data.DocTypeOther})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Brand != nil || got.Model != nil || got.SerialNumber != nil {
		t.Errorf("optional name fields not nil: brand=%v model=%v serial=%v", got.Brand, got.Model, got.SerialNumber)
	}
	if got.NormSerial != nil || got.NormBrand != nil || got.NormModel != nil {
		t.Errorf("norm fields not nil: serial=%v brand=%v model=%v", got.NormSerial, got.NormBrand, got.NormModel)
	}
	if got.PurchaseDate != nil || got.WarrantyEnd != nil {
		t.Errorf("dates not nil: purchase=%v warranty=%v", got.PurchaseDate, got.WarrantyEnd)
	}
	if got.Price != nil || got.Currency != nil {
		t.Errorf("price/currency not nil: price=%v currency=%v", got.Price, got.Currency)
	}
	if len(got.Metadata) != 0 {
		t.Errorf("Metadata = %v, want empty", got.Metadata)
	}
	if !got.CreatedAt.IsZero() {
		// CreatedAt is set by DB default now(); just ensure it is non-zero.
		t.Logf("CreatedAt = %v", got.CreatedAt)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("CreatedAt/UpdatedAt = %v/%v, want DB defaults (non-zero)", got.CreatedAt, got.UpdatedAt)
	}
}

func TestPriceRoundTrip(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	created, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
		a.Price = strPtr("39999.99")
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Price == nil {
		t.Fatal("Price is nil, want \"39999.99\"")
	}
	if *got.Price != "39999.99" {
		t.Errorf("Price = %q, want %q", *got.Price, "39999.99")
	}
}

func TestListStableOrder(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	for i, serial := range []string{"SN-A", "SN-B", "SN-C"} {
		_, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
			a.SerialNumber = strPtr(serial)
			a.Model = strPtr(fmt.Sprintf("M%d", i))
		}))
		if err != nil {
			t.Fatalf("Create[%s]: %v", serial, err)
		}
	}

	first, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List first: %v", err)
	}
	second, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List second: %v", err)
	}
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("List lengths = %d, %d, want 3, 3", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("order differs at %d: %s vs %s", i, first[i].ID, second[i].ID)
		}
	}

	// Empty table: List returns non-nil empty slice.
	truncate(t)
	empty, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if empty == nil {
		t.Error("List on empty table = nil, want non-nil empty slice")
	}
	if len(empty) != 0 {
		t.Errorf("List on empty table = %d rows, want 0", len(empty))
	}
}

func TestGetByIDNotFound(t *testing.T) {
	truncate(t)
	randomID := "00000000-0000-0000-0000-000000000000"
	if _, err := repo.GetByID(ctxWithTenant(tenantA), randomID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(random): err = %v, want ErrNotFound", err)
	}
}

func TestFindBySerial(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	created, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
		a.SerialNumber = strPtr("  wm-2024-001  ")
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindBySerial(ctx, "WM-2024-001")
	if err != nil {
		t.Fatalf("FindBySerial: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("FindBySerial ID = %q, want %q", got.ID, created.ID)
	}

	if _, err := repo.FindBySerial(ctx, "NONEXISTENT"); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("FindBySerial(NONEXISTENT): err = %v, want ErrNotFound", err)
	}
}

func TestFindByBrandModel(t *testing.T) {
	truncate(t)
	ctx := ctxWithTenant(tenantA)

	created, err := repo.Create(ctx, testAsset())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByBrandModel(ctx, "samsung", "wf80a")
	if err != nil {
		t.Fatalf("FindByBrandModel: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("FindByBrandModel ID = %q, want %q", got.ID, created.ID)
	}

	cases := []struct {
		name    string
		brand   string
		model   string
		wantErr bool
	}{
		{"wrong model", "samsung", "nonexistent", true},
		{"wrong brand", "nonexistent", "wf80a", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.FindByBrandModel(ctx, tc.brand, tc.model)
			if tc.wantErr && !errors.Is(err, data.ErrNotFound) {
				t.Errorf("FindByBrandModel(%q, %q): err = %v, want ErrNotFound", tc.brand, tc.model, err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("FindByBrandModel(%q, %q): unexpected err %v", tc.brand, tc.model, err)
			}
		})
	}
}

func TestCreateOnConflictSerial(t *testing.T) {
	ctxA := ctxWithTenant(tenantA)
	ctxB := ctxWithTenant(tenantB)

	t.Run("sequential conflict returns existing", func(t *testing.T) {
		truncate(t)

		first, err := repo.Create(ctxA, testAsset(func(a *data.Asset) {
			a.SerialNumber = strPtr("SN-1")
		}))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		asset, created, err := repo.CreateOnConflictSerial(ctxA, testAsset(func(a *data.Asset) {
			a.SerialNumber = strPtr("SN-1")
			a.Brand = strPtr("OtherBrand")
		}))
		if err != nil {
			t.Fatalf("CreateOnConflictSerial: %v", err)
		}
		if created {
			t.Error("created = true, want false (conflict)")
		}
		if asset.ID != first.ID {
			t.Errorf("asset.ID = %q, want existing %q", asset.ID, first.ID)
		}
		if *asset.Brand != "Samsung" {
			t.Errorf("brand = %q, want unchanged %q", *asset.Brand, "Samsung")
		}
	})

	t.Run("concurrent inserts create exactly one", func(t *testing.T) {
		truncate(t)

		var wg sync.WaitGroup
		results := make([]struct {
			asset   data.Asset
			created bool
			err     error
		}, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				a, c, err := repo.CreateOnConflictSerial(ctxA, testAsset(func(x *data.Asset) {
					x.SerialNumber = strPtr("SN-2")
				}))
				results[i] = struct {
					asset   data.Asset
					created bool
					err     error
				}{a, c, err}
			}(i)
		}
		wg.Wait()

		var createdCount int
		var id string
		for i, r := range results {
			if r.err != nil {
				t.Fatalf("goroutine %d: %v", i, r.err)
			}
			if r.created {
				createdCount++
			}
			if i == 0 {
				id = r.asset.ID
			} else if r.asset.ID != id {
				t.Errorf("concurrent results disagree on ID: %q vs %q", r.asset.ID, id)
			}
		}
		if createdCount != 1 {
			t.Errorf("createdCount = %d, want exactly 1", createdCount)
		}
	})

	t.Run("cross-tenant same serial both succeed", func(t *testing.T) {
		truncate(t)

		a1, created1, err := repo.CreateOnConflictSerial(ctxA, testAsset(func(x *data.Asset) {
			x.SerialNumber = strPtr("SN-3")
		}))
		if err != nil {
			t.Fatalf("CreateOnConflictSerial(A): %v", err)
		}
		a2, created2, err := repo.CreateOnConflictSerial(ctxB, testAsset(func(x *data.Asset) {
			x.SerialNumber = strPtr("SN-3")
		}))
		if err != nil {
			t.Fatalf("CreateOnConflictSerial(B): %v", err)
		}
		if !created1 || !created2 {
			t.Errorf("created = (%v, %v), want (true, true)", created1, created2)
		}
		if a1.ID == a2.ID {
			t.Error("cross-tenant inserts share ID, want distinct")
		}
		if a1.TenantID != tenantA || a2.TenantID != tenantB {
			t.Errorf("tenants = (%q, %q), want (%q, %q)", a1.TenantID, a2.TenantID, tenantA, tenantB)
		}
	})
}

func TestUpdateFields(t *testing.T) {
	ctx := ctxWithTenant(tenantA)

	t.Run("single field update leaves others untouched", func(t *testing.T) {
		truncate(t)

		purchase := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		created, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
			a.PurchaseDate = &purchase
			a.Price = strPtr("100.50")
		}))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		lg := "LG"
		updated, err := repo.UpdateFields(ctx, created.ID, data.UpdateFields{Brand: &lg})
		if err != nil {
			t.Fatalf("UpdateFields: %v", err)
		}
		if updated.Brand == nil || *updated.Brand != "LG" {
			t.Errorf("Brand = %v, want LG", updated.Brand)
		}
		if updated.NormBrand == nil || *updated.NormBrand != "lg" {
			t.Errorf("NormBrand = %v, want lg", updated.NormBrand)
		}
		if updated.Model == nil || *updated.Model != "WF80A" {
			t.Errorf("Model changed: %v, want WF80A", updated.Model)
		}
		if updated.SerialNumber == nil || *updated.SerialNumber != "WM-2024-001" {
			t.Errorf("SerialNumber changed: %v", updated.SerialNumber)
		}
		if updated.Price == nil || *updated.Price != "100.50" {
			t.Errorf("Price changed: %v, want 100.50", updated.Price)
		}
	})

	t.Run("metadata shallow merge", func(t *testing.T) {
		truncate(t)

		created, err := repo.Create(ctx, testAsset(func(a *data.Asset) {
			a.Metadata = map[string]any{"old_key": "old_val"}
		}))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		updated, err := repo.UpdateFields(ctx, created.ID, data.UpdateFields{
			Metadata: map[string]any{"new_key": "val"},
		})
		if err != nil {
			t.Fatalf("UpdateFields: %v", err)
		}
		if updated.Metadata["old_key"] != "old_val" {
			t.Errorf("Metadata[old_key] = %v, want old_val (merged)", updated.Metadata["old_key"])
		}
		if updated.Metadata["new_key"] != "val" {
			t.Errorf("Metadata[new_key] = %v, want val", updated.Metadata["new_key"])
		}
		if len(updated.Metadata) != 2 {
			t.Errorf("Metadata = %v, want 2 keys", updated.Metadata)
		}
	})

	t.Run("all nil fields is a no-op", func(t *testing.T) {
		truncate(t)

		created, err := repo.Create(ctx, testAsset())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		updated, err := repo.UpdateFields(ctx, created.ID, data.UpdateFields{})
		if err != nil {
			t.Fatalf("UpdateFields: %v", err)
		}
		assertAssetEqual(t, "asset", updated, created)
		if !updated.UpdatedAt.Equal(created.UpdatedAt) {
			t.Errorf("UpdatedAt changed on no-op: %v vs %v", updated.UpdatedAt, created.UpdatedAt)
		}
	})

	t.Run("doc_type update", func(t *testing.T) {
		truncate(t)

		created, err := repo.Create(ctx, testAsset())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.DocType != data.DocTypeInvoice {
			t.Fatalf("seed DocType = %q, want invoice", created.DocType)
		}

		amc := data.DocTypeAMC
		updated, err := repo.UpdateFields(ctx, created.ID, data.UpdateFields{DocType: &amc})
		if err != nil {
			t.Fatalf("UpdateFields: %v", err)
		}
		if updated.DocType != data.DocTypeAMC {
			t.Errorf("DocType = %q, want amc", updated.DocType)
		}
	})
}

func TestTenantLessContext(t *testing.T) {
	truncate(t)
	bg := context.Background()

	created, err := repo.Create(ctxWithTenant(tenantA), testAsset())
	if err != nil {
		t.Fatalf("setup Create: %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			_, err := repo.Create(bg, testAsset())
			return err
		}},
		{"CreateOnConflictSerial", func() error {
			_, _, err := repo.CreateOnConflictSerial(bg, testAsset())
			return err
		}},
		{"GetByID", func() error {
			_, err := repo.GetByID(bg, created.ID)
			return err
		}},
		{"List", func() error {
			_, err := repo.List(bg)
			return err
		}},
		{"FindBySerial", func() error {
			_, err := repo.FindBySerial(bg, "X")
			return err
		}},
		{"FindByBrandModel", func() error {
			_, err := repo.FindByBrandModel(bg, "x", "y")
			return err
		}},
		{"UpdateFields", func() error {
			_, err := repo.UpdateFields(bg, created.ID, data.UpdateFields{})
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
