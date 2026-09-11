package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
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
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
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

// registryPool returns the p_generic pool bound to the user registry. It is
// lazily initialized inside genericOnce alongside the repositories.
func registryPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	genericRepos(t) // ensure the pool is initialized
	return genericPool
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

	created, err := assets.Create(context.Background(), testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := assets.Get(context.Background(), created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertAssetEqual(t, "asset", got, created)
}

func TestGenericAssetListWithUserAndWhere(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	serials := []string{"WM-2024-001", "WM-2024-002", "WM-2024-003"}
	for i, cat := range []string{entity.AssetCategoryAppliance, entity.AssetCategoryFurniture, entity.AssetCategoryAppliance} {
		serial := serials[i]
		// Distinct serial per row: 00001_init added the partial unique index
		// partial unique index on (owner_id, norm_serial).
		if _, err := assets.Create(ctx, testAsset(func(a *entity.Asset) {
			a.AssetCategory = &cat
			a.SerialNumber = &serial
		}), repo.Owner(userA)); err != nil {
			t.Fatalf("Create(%s): %v", cat, err)
		}
	}

	cases := []struct {
		name string
		op   string
		val  any
		want int
	}{
		{"equals appliance", "=", entity.AssetCategoryAppliance, 2},
		{"equals furniture", "=", entity.AssetCategoryFurniture, 1},
		{"equals vehicle (none)", "=", entity.AssetCategoryVehicle, 0},
		{"not equals appliance", "!=", entity.AssetCategoryAppliance, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := assets.List(ctx, repo.Owner(userA), repo.Where("asset_category", tc.op, tc.val))
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("List = %d rows, want %d", len(got), tc.want)
			}
			for _, a := range got {
				if tc.op == "=" && (a.AssetCategory == nil || *a.AssetCategory != tc.val) {
					t.Errorf("row AssetCategory = %v, want %v", a.AssetCategory, tc.val)
				}
			}
		})
	}
}

func TestGenericAssetCreate(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	created, err := assets.Create(context.Background(), testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("Create returned empty ID, want DB-generated uuid")
	}
	if created.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", created.OwnerID, userA)
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

	created, err := assets.Create(ctx, testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Only ID + Brand set; every other field is zero.
	lg := "LG"
	partial := entity.Asset{ID: created.ID, Brand: &lg}
	updated, err := assets.Update(ctx, partial, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Brand == nil || *updated.Brand != "LG" {
		t.Errorf("Brand = %v, want LG", updated.Brand)
	}

	// Re-fetch and verify unchanged fields survived the partial update.
	got, err := assets.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertPtrEqual(t, "Model", got.Model, created.Model)
	assertPtrEqual(t, "SerialNumber", got.SerialNumber, created.SerialNumber)
	assertTimePtrEqual(t, "PurchaseDate", got.PurchaseDate, created.PurchaseDate)
	assertPtrEqual(t, "AssetCategory", got.AssetCategory, created.AssetCategory)
}

func TestGenericAssetDelete(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	created, err := assets.Create(ctx, testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := assets.Delete(ctx, created.ID, repo.Owner(userA)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := assets.Get(ctx, created.ID, repo.Owner(userA)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

// TestGenericDocumentUpdateClearsRealColumn pins the fix for a nil bare-column
// repo.Set: `asset_id = NULL` must be emitted as a SQL literal (no bound
// parameter). The stale implementation appended a nil arg for the NULL, which
// shifted every later placeholder and left an untyped $N → SQLSTATE 42P18
// "could not determine data type of parameter $N". The documents delete path
// (soft-delete + detach) is the exact caller this regression guards.
func TestGenericDocumentUpdateClearsRealColumn(t *testing.T) {
	assets, sources, docs := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	asset, err := assets.Create(ctx, testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	src, err := sources.Create(ctx, testSource(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	created, err := docs.Create(ctx, entity.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         entity.DocTypeInvoice,
		ExtractedFields: map[string]any{},
		// 00001_init requires documents.raw_extraction NOT NULL.
		RawExtraction: "raw extraction fixture",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create document: %v", err)
	}
	if created.AssetID != asset.ID {
		t.Fatalf("created.AssetID = %q, want %q", created.AssetID, asset.ID)
	}

	// The delete path: soft-delete (set deleted_at) + clear the nullable real
	// column asset_id via repo.Set. This is exactly what the documents
	// DeleteDocument handler does.
	now := time.Now()
	created.DeletedAt = &now
	updated, err := docs.Update(ctx, created, repo.Owner(userA), repo.Set("asset_id", nil))
	if err != nil {
		t.Fatalf("Update (soft-delete + repo.Set(asset_id, nil)): %v", err)
	}
	if updated.AssetID != "" {
		t.Errorf("updated.AssetID = %q, want \"\" (cleared to NULL)", updated.AssetID)
	}
	if updated.DeletedAt == nil {
		t.Error("updated.DeletedAt is nil, want set (soft-delete persisted)")
	}
	// The payload merge must survive the real-column changes.
	if updated.DocType != entity.DocTypeInvoice {
		t.Errorf("updated.DocType = %q, want %q (payload preserved)", updated.DocType, entity.DocTypeInvoice)
	}
	got, err := docs.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AssetID != "" {
		t.Errorf("re-fetched AssetID = %q, want \"\" (NULL)", got.AssetID)
	}
	if got.DeletedAt == nil {
		t.Error("re-fetched DeletedAt is nil, want set (soft-delete persisted)")
	}
}

func TestGenericSourceCreateGetRoundTrip(t *testing.T) {
	_, sources, _ := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	want := testSource()
	want.UploadedAt = time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)
	created, err := sources.Create(ctx, want, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}

	got, err := sources.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertSourceEqual(t, "source", got, created)
}

func TestGenericDocumentListOrderByLimit(t *testing.T) {
	assets, sources, docs := genericRepos(t)
	truncateGeneric(t)
	ctx := context.Background()

	asset, err := assets.Create(ctx, testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	for i := 0; i < 3; i++ {
		src, err := sources.Create(ctx, testSource(), repo.Owner(userA))
		if err != nil {
			t.Fatalf("seed source[%d]: %v", i, err)
		}
		if _, err := docs.Create(ctx, entity.Document{
			SourceID:        src.ID,
			AssetID:         asset.ID,
			DocType:         entity.DocTypeInvoice,
			ExtractedFields: map[string]any{},
			// 00001_init requires documents.raw_extraction NOT NULL.
			RawExtraction: "raw extraction fixture",
		}, repo.Owner(userA)); err != nil {
			t.Fatalf("Create document[%d]: %v", i, err)
		}
		time.Sleep(time.Millisecond) // distinct created_at values
	}

	got, err := docs.List(ctx, repo.Owner(userA), repo.OrderBy("created_at"), repo.Limit(2))
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

func TestGenericUserIsolation(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	created, err := assets.Create(context.Background(), testAsset(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get under the other user must not see the row.
	if _, err := assets.Get(context.Background(), created.ID, repo.Owner(userB)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get(other user): err = %v, want ErrNotFound", err)
	}

	// List under the other user must be empty.
	list, err := assets.List(context.Background(), repo.Owner(userB))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List(other user) = %d rows, want 0", len(list))
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
			created, err = repos.Assets.Create(ctx, testAsset(), repo.Owner(userA))
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
		got, err := assets.Get(context.Background(), createdID, repo.Owner(userA))
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
			if _, err := repos.Assets.Create(ctx, testAsset(), repo.Owner(userA)); err != nil {
				return err
			}
			return forcedErr
		})
		if !errors.Is(err, forcedErr) {
			t.Fatalf("InTx: err = %v, want %q", err, forcedErr)
		}

		// Rolled back: no rows visible after the transaction.
		assets, _, _ := genericRepos(t)
		list, err := assets.List(context.Background(), repo.Owner(userA))
		if err != nil {
			t.Fatalf("List after rollback: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("List after rollback = %d rows, want 0 (rolled back)", len(list))
		}
	})
}

// TestGenericCtxFallback verifies that the repository resolves the user
// from context when no explicit repo.Owner option is provided.
func TestGenericCtxFallback(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	ctx := user.WithUser(context.Background(), userA)

	// Create via ctx user (no repo.Owner option).
	created, err := assets.Create(ctx, testAsset())
	if err != nil {
		t.Fatalf("Create (ctx user): %v", err)
	}
	if created.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", created.OwnerID, userA)
	}

	// Get via ctx user.
	got, err := assets.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get (ctx user): %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get ID = %q, want %q", got.ID, created.ID)
	}

	// List via ctx user.
	list, err := assets.List(ctx)
	if err != nil {
		t.Fatalf("List (ctx user): %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %d rows, want 1", len(list))
	}

	// Delete via ctx user.
	if err := assets.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete (ctx user): %v", err)
	}
	if _, err := assets.Get(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

// TestGenericNoUserErr verifies that a repository call with neither an
// explicit option nor a context user returns ErrNoUser.
func TestGenericNoUserErr(t *testing.T) {
	assets, _, _ := genericRepos(t)
	truncateGeneric(t)

	ctx := context.Background() // no user

	if _, err := assets.Create(ctx, testAsset()); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("Create: err = %v, want ErrNoUser", err)
	}
	if _, err := assets.Get(ctx, "some-id"); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("Get: err = %v, want ErrNoUser", err)
	}
	if _, err := assets.List(ctx); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("List: err = %v, want ErrNoUser", err)
	}
	if _, err := assets.Update(ctx, entity.Asset{ID: "some-id"}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("Update: err = %v, want ErrNoUser", err)
	}
	if err := assets.Delete(ctx, "some-id"); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("Delete: err = %v, want ErrNoUser", err)
	}
}
