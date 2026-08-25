package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

const testSchemaGeneric = "p_generic"

var (
	genericOnce    sync.Once
	genericPool    *pgxpool.Pool
	genericAssets  repo.Repository[entity.Asset]
	genericSources repo.Repository[entity.Source]
	genericDocs    repo.Repository[entity.Document]
	genericFactory *repo.Factory
	genericInitErr error
)

// genericRepos returns the generic repositories bound to the p_generic test
// schema. Uses the sync.Once lazy init pattern (see sources_test.go): the
// pool, repositories, and factory are opened and initialized exactly once per
// test binary run.
func genericRepos(t *testing.T) (repo.Repository[entity.Asset], repo.Repository[entity.Source], repo.Repository[entity.Document]) {
	t.Helper()
	genericOnce.Do(func() {
		genericPool, genericInitErr = openTestPool(t, testSchemaGeneric)
		if genericInitErr != nil {
			return
		}
		genericAssets = postgres.NewAssetRepository(genericPool)
		genericSources = postgres.NewSourceRepository(genericPool)
		genericDocs = postgres.NewDocumentRepository(genericPool)
		genericFactory = postgres.NewFactory(genericPool)
	})
	if genericInitErr != nil {
		t.Fatalf("init generic test pool: %v", genericInitErr)
	}
	return genericAssets, genericSources, genericDocs
}

// genericFactoryRepo returns the repo.Factory built over the p_generic pool.
// It is lazily initialized inside genericOnce alongside the repositories.
func genericFactoryRepo(t *testing.T) *repo.Factory {
	t.Helper()
	genericRepos(t) // ensure the pool is initialized
	return genericFactory
}

// truncateGeneric clears all test data on the generic pool.
// Call at the top of each write test.
func truncateGeneric(t *testing.T) {
	t.Helper()
	genericRepos(t) // ensure the pool is initialized
	if _, err := genericPool.Exec(context.Background(), `TRUNCATE documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate generic: %v", err)
	}
}

func TestGenericAssetGetByID(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	created, err := assets.Create(context.Background(), testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := assets.Get(context.Background(), created.ID, repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertAssetEqual(t, "asset", got, created)
}

func TestGenericAssetListWithTenantAndWhere(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	for _, dt := range []string{entity.DocTypeInvoice, entity.DocTypeAMC, entity.DocTypeInvoice} {
		if _, err := assets.Create(ctx, testAsset(func(a *entity.Asset) {
			a.DocType = dt
		}), repo.Tenant(tenantA)); err != nil {
			t.Fatalf("Create(%s): %v", dt, err)
		}
	}

	cases := []struct {
		name string
		op   string
		val  any
		want int
	}{
		{"equals invoice", "=", entity.DocTypeInvoice, 2},
		{"equals amc", "=", entity.DocTypeAMC, 1},
		{"equals warranty (none)", "=", entity.DocTypeWarranty, 0},
		{"not equals invoice", "!=", entity.DocTypeInvoice, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := assets.List(ctx, repo.Tenant(tenantA), repo.Where("doc_type", tc.op, tc.val))
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("List = %d rows, want %d", len(got), tc.want)
			}
			for _, a := range got {
				if tc.op == "=" && a.DocType != tc.val {
					t.Errorf("row DocType = %q, want %q", a.DocType, tc.val)
				}
			}
		})
	}
}

func TestGenericAssetCreate(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	created, err := assets.Create(context.Background(), testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("Create returned empty ID, want DB-generated uuid")
	}
	if created.TenantID != tenantA {
		t.Errorf("TenantID = %q, want %q", created.TenantID, tenantA)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want DB default now()")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want DB default now()")
	}
}

func TestGenericAssetUpdatePartial(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	created, err := assets.Create(ctx, testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Only ID + Brand set; every other field is zero.
	lg := "LG"
	partial := entity.Asset{ID: created.ID, Brand: &lg}
	updated, err := assets.Update(ctx, partial, repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Brand == nil || *updated.Brand != "LG" {
		t.Errorf("Brand = %v, want LG", updated.Brand)
	}

	// Re-fetch and verify unchanged fields survived the partial update.
	got, err := assets.Get(ctx, created.ID, repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertPtrEqual(t, "Model", got.Model, created.Model)
	assertPtrEqual(t, "SerialNumber", got.SerialNumber, created.SerialNumber)
	assertTimePtrEqual(t, "PurchaseDate", got.PurchaseDate, created.PurchaseDate)
	if got.DocType != created.DocType {
		t.Errorf("DocType = %q, want unchanged %q", got.DocType, created.DocType)
	}
}

func TestGenericAssetDelete(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	created, err := assets.Create(ctx, testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := assets.Delete(ctx, created.ID, repo.Tenant(tenantA)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := assets.Get(ctx, created.ID, repo.Tenant(tenantA)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

func TestGenericSourceCreateGetRoundTrip(t *testing.T) {
	_, sources, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	want := testSource()
	want.UploadedAt = time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)
	created, err := sources.Create(ctx, want, repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}

	got, err := sources.Get(ctx, created.ID, repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertSourceEqual(t, "source", got, created)
}

func TestGenericDocumentListOrderByLimit(t *testing.T) {
	assets, sources, docs := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	asset, err := assets.Create(ctx, testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	for i := 0; i < 3; i++ {
		src, err := sources.Create(ctx, testSource(), repo.Tenant(tenantA))
		if err != nil {
			t.Fatalf("seed source[%d]: %v", i, err)
		}
		if _, err := docs.Create(ctx, entity.Document{
			SourceID:        src.ID,
			AssetID:         asset.ID,
			DocType:         entity.DocTypeInvoice,
			ExtractedFields: map[string]any{},
		}, repo.Tenant(tenantA)); err != nil {
			t.Fatalf("Create document[%d]: %v", i, err)
		}
		time.Sleep(time.Millisecond) // distinct created_at values
	}

	got, err := docs.List(ctx, repo.Tenant(tenantA), repo.OrderBy("created_at"), repo.Limit(2))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List = %d rows, want exactly 2", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatedAt.After(got[i].CreatedAt) {
			t.Errorf("order violated at %d: %v after %v", i, got[i-1].CreatedAt, got[i].CreatedAt)
		}
	}
}

func TestGenericTenantIsolation(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	created, err := assets.Create(context.Background(), testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get under the other tenant must not see the row.
	if _, err := assets.Get(context.Background(), created.ID, repo.Tenant(tenantB)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get(other tenant): err = %v, want ErrNotFound", err)
	}

	// List under the other tenant must be empty.
	list, err := assets.List(context.Background(), repo.Tenant(tenantB))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List(other tenant) = %d rows, want 0", len(list))
	}
}

func TestGenericFactoryInTx(t *testing.T) {
	f := genericFactoryRepo(t)

	t.Run("commits on success", func(t *testing.T) {
		truncateGeneric(t)

		var createdID string
		err := f.InTx(context.Background(), func(ctx context.Context, repos *repo.Repos) error {
			var err error
			var created entity.Asset
			created, err = repos.Assets.Create(ctx, testAsset(), repo.Tenant(tenantA))
			if err != nil {
				return err
			}
			createdID = created.ID
			return nil
		})
		if err != nil {
			t.Fatalf("InTx: %v", err)
		}

		// Committed: visible after the transaction via the pool-bound repo.
		assets, _, _ := genericRepos(t)
		got, err := assets.Get(context.Background(), createdID, repo.Tenant(tenantA))
		if err != nil {
			t.Fatalf("Get after commit: %v, want visible row", err)
		}
		if got.ID != createdID {
			t.Errorf("Get ID = %q, want %q", got.ID, createdID)
		}
	})

	t.Run("rolls back on error", func(t *testing.T) {
		truncateGeneric(t)

		forcedErr := errors.New("forced rollback")
		err := f.InTx(context.Background(), func(ctx context.Context, repos *repo.Repos) error {
			if _, err := repos.Assets.Create(ctx, testAsset(), repo.Tenant(tenantA)); err != nil {
				return err
			}
			return forcedErr
		})
		if !errors.Is(err, forcedErr) {
			t.Fatalf("InTx: err = %v, want %q", err, forcedErr)
		}

		// Rolled back: no rows visible after the transaction.
		assets, _, _ := genericRepos(t)
		list, err := assets.List(context.Background(), repo.Tenant(tenantA))
		if err != nil {
			t.Fatalf("List after rollback: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("List after rollback = %d rows, want 0 (rolled back)", len(list))
		}
	})
}
