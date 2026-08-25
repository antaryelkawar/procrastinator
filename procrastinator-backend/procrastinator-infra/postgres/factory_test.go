package postgres_test

import (
	"context"
	"errors"
	"testing"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-infra/postgres"
)

// getFactory returns the pgRepoFactory built over the package-level pool.
// The pool is set up in TestMain (assets_test.go) before any test runs.
func getFactory() data.RepoFactory {
	return postgres.NewRepoFactory(pool)
}

// truncateFactory clears all test data. Call at the top of each write test.
func truncateFactory(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate factory: %v", err)
	}
}

// TestRepoFactory_ExposesRepos verifies that NewRepoFactory returns a factory
// whose three accessors each return a non-nil repository of the correct type.
func TestRepoFactory_ExposesRepos(t *testing.T) {
	f := getFactory()
	truncateFactory(t)

	var assetRepo data.AssetRepository = f.AssetRepo()
	var srcRepo data.SourceRepository = f.SourceRepo()
	var docRepo data.DocumentRepository = f.DocumentRepo()

	if assetRepo == nil {
		t.Error("AssetRepo() = nil, want non-nil data.AssetRepository")
	}
	if srcRepo == nil {
		t.Error("SourceRepo() = nil, want non-nil data.SourceRepository")
	}
	if docRepo == nil {
		t.Error("DocumentRepo() = nil, want non-nil data.DocumentRepository")
	}
}

// TestInTransaction_CommitsOnSuccess verifies that an InTransaction call whose
// fn returns nil commits the writes made through the tx-bound repos: the
// created asset must be visible via the pool-bound factory afterwards.
func TestInTransaction_CommitsOnSuccess(t *testing.T) {
	f := getFactory()
	truncateFactory(t)
	ctxA := ctxWithTenant(tenantA)

	err := f.InTransaction(ctxA, func(ctx context.Context) error {
		txFactory, ok := data.RepoFactoryFromContext(ctx)
		if !ok {
			return errors.New("no factory in context")
		}
		_, err := txFactory.AssetRepo().Create(ctx, testAsset())
		return err
	})
	if err != nil {
		t.Fatalf("InTransaction: %v", err)
	}

	// After the committed transaction, the asset must be visible.
	assets, err := f.AssetRepo().List(ctxA)
	if err != nil {
		t.Fatalf("List after commit: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("List after commit = %d rows, want 1", len(assets))
	}
	if assets[0].TenantID != tenantA {
		t.Errorf("asset TenantID = %q, want %q", assets[0].TenantID, tenantA)
	}
}

// TestInTransaction_RollsBackOnError verifies that an InTransaction call whose
// fn returns an error rolls back all writes: no asset is visible afterwards,
// and the fn's error is propagated to the caller.
func TestInTransaction_RollsBackOnError(t *testing.T) {
	f := getFactory()
	truncateFactory(t)
	ctxA := ctxWithTenant(tenantA)

	forcedErr := errors.New("forced rollback")
	err := f.InTransaction(ctxA, func(ctx context.Context) error {
		txFactory, ok := data.RepoFactoryFromContext(ctx)
		if !ok {
			return errors.New("no factory in context")
		}
		if _, err := txFactory.AssetRepo().Create(ctx, testAsset()); err != nil {
			return err
		}
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Errorf("InTransaction: err = %v, want %q", err, forcedErr)
	}

	// After the rolled-back transaction, no assets should be visible.
	assets, err := f.AssetRepo().List(ctxA)
	if err != nil {
		t.Fatalf("List after rollback: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("List after rollback = %d rows, want 0 (rolled back)", len(assets))
	}
}

// TestInTransaction_SharedTxWithinFn verifies that all repos obtained from the
// tx-bound factory share one transaction: asset + source + document inserted
// in the same fn commit together, and roll back together when the fn errors.
func TestInTransaction_SharedTxWithinFn(t *testing.T) {
	f := getFactory()

	t.Run("commits together", func(t *testing.T) {
		truncateFactory(t)
		ctxA := ctxWithTenant(tenantA)

		err := f.InTransaction(ctxA, func(ctx context.Context) error {
			txFactory, ok := data.RepoFactoryFromContext(ctx)
			if !ok {
				return errors.New("no factory in context")
			}

			asset, err := txFactory.AssetRepo().Create(ctx, testAsset())
			if err != nil {
				return err
			}

			src, err := txFactory.SourceRepo().Create(ctx, testSource())
			if err != nil {
				return err
			}

			_, err = txFactory.DocumentRepo().Create(ctx, data.Document{
				SourceID:        src.ID,
				AssetID:         asset.ID,
				DocType:         data.DocTypeInvoice,
				ExtractedFields: map[string]any{"invoice_no": "INV-99"},
			})
			return err
		})
		if err != nil {
			t.Fatalf("InTransaction: %v", err)
		}

		assets, err := f.AssetRepo().List(ctxA)
		if err != nil {
			t.Fatalf("List assets: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("assets after commit = %d, want 1", len(assets))
		}

		docs, err := f.DocumentRepo().ListByAsset(ctxA, assets[0].ID)
		if err != nil {
			t.Fatalf("ListByAsset: %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("documents after commit = %d, want 1", len(docs))
		}
		if docs[0].Document.DocType != data.DocTypeInvoice {
			t.Errorf("document DocType = %q, want %q", docs[0].Document.DocType, data.DocTypeInvoice)
		}
	})

	t.Run("rolls back together", func(t *testing.T) {
		truncateFactory(t)
		ctxA := ctxWithTenant(tenantA)

		forcedErr := errors.New("forced rollback after all inserts")
		err := f.InTransaction(ctxA, func(ctx context.Context) error {
			txFactory, ok := data.RepoFactoryFromContext(ctx)
			if !ok {
				return errors.New("no factory in context")
			}

			asset, err := txFactory.AssetRepo().Create(ctx, testAsset())
			if err != nil {
				return err
			}

			src, err := txFactory.SourceRepo().Create(ctx, testSource())
			if err != nil {
				return err
			}

			if _, err := txFactory.DocumentRepo().Create(ctx, data.Document{
				SourceID:        src.ID,
				AssetID:         asset.ID,
				DocType:         data.DocTypeInvoice,
				ExtractedFields: map[string]any{},
			}); err != nil {
				return err
			}
			return forcedErr
		})
		if !errors.Is(err, forcedErr) {
			t.Fatalf("InTransaction: err = %v, want %v", err, forcedErr)
		}

		// Neither asset nor document survives the rollback.
		assets, err := f.AssetRepo().List(ctxA)
		if err != nil {
			t.Fatalf("List assets: %v", err)
		}
		if len(assets) != 0 {
			t.Errorf("assets after rollback = %d, want 0 (all rolled back together)", len(assets))
		}
	})
}

// TestInTransaction_TenantScoped verifies that writes made inside InTransaction
// are tenant-scoped: the asset created under tenantA is visible to tenantA
// after commit but invisible to tenantB.
func TestInTransaction_TenantScoped(t *testing.T) {
	f := getFactory()
	truncateFactory(t)
	ctxA := ctxWithTenant(tenantA)
	ctxB := ctxWithTenant(tenantB)

	err := f.InTransaction(ctxA, func(ctx context.Context) error {
		txFactory, ok := data.RepoFactoryFromContext(ctx)
		if !ok {
			return errors.New("no factory in context")
		}
		_, err := txFactory.AssetRepo().Create(ctx, testAsset())
		return err
	})
	if err != nil {
		t.Fatalf("InTransaction: %v", err)
	}

	// tenantA sees the asset.
	assetsA, err := f.AssetRepo().List(ctxA)
	if err != nil {
		t.Fatalf("List(tenantA): %v", err)
	}
	if len(assetsA) != 1 {
		t.Errorf("List(tenantA) = %d rows, want 1", len(assetsA))
	}

	// tenantB does not see the asset.
	assetsB, err := f.AssetRepo().List(ctxB)
	if err != nil {
		t.Fatalf("List(tenantB): %v", err)
	}
	if len(assetsB) != 0 {
		t.Errorf("List(tenantB) = %d rows, want 0 (tenant isolation)", len(assetsB))
	}
}
