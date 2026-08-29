package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

// pgFKViolation reports whether err is a PostgreSQL foreign key violation
// (SQLSTATE 23503).
func pgFKViolation(t *testing.T, op string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: succeeded, want FK violation (23503)", op)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("%s: err = %v, want foreign key violation (23503)", op, err)
	}
}

// TestTenantRegistry_Has verifies the Has method:
// - true for the seeded test-tenant (migration 00002 seeds it)
// - false for an unknown tenant
// - true after inserting a registry row
func TestTenantRegistry_Has(t *testing.T) {
	pool := registryPool(t)
	ctx := context.Background()
	registry := postgres.NewTenantRegistry(pool)

	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"seeded tenant is registered", tenantA, true},
		{"unknown tenant is not registered", "unknown-tenant", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := registry.Has(ctx, tc.id)
			if err != nil {
				t.Fatalf("Has(%q): %v", tc.id, err)
			}
			if got != tc.want {
				t.Errorf("Has(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}

	t.Run("newly inserted tenant is registered", func(t *testing.T) {
		// ON CONFLICT keeps the test re-runnable: the shared p_generic
		// schema persists across runs, so the row may already exist.
		if _, err := pool.Exec(ctx, `INSERT INTO tenants (id) VALUES ('new-tenant') ON CONFLICT (id) DO NOTHING`); err != nil {
			t.Fatalf("INSERT tenant: %v", err)
		}
		got, err := registry.Has(ctx, "new-tenant")
		if err != nil {
			t.Fatalf("Has(new-tenant): %v", err)
		}
		if !got {
			t.Error("Has(new-tenant) = false, want true")
		}
	})
}

// TestTenantRegistry_FKEforcement verifies the registry's FK enforcement:
// tenant-owned rows must reference a registered tenant, so inserting an asset
// under an unregistered tenant fails with SQLSTATE 23503.
func TestTenantRegistry_FKEforcement(t *testing.T) {
	t.Parallel()
	pool := registryPool(t)
	ctx := context.Background()

	if err := ensureTenants(ctx, pool, tenantA); err != nil {
		t.Fatalf("ensureTenants: %v", err)
	}

	// The insert runs inside a tenant-bound transaction: with the
	// non-superuser app role (design D4) RLS actually applies, and the
	// policy's WITH CHECK passes only when the bound tenant equals the
	// row's tenant_id. Binding to the unregistered id lets the FK check
	// (the registry enforcement under test) run and fail with 23503.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) // no-op on the expected error path
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, "unregistered-tenant"); err != nil {
		t.Fatalf("bind tenant: %v", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO assets (id, tenant_id, doc_type) VALUES (gen_random_uuid(), 'unregistered-tenant', 'invoice')`)
	pgFKViolation(t, "INSERT asset for unregistered tenant", err)
}

// TestTenantRegistry_DeleteWithData verifies the ON DELETE RESTRICT
// behavior: a registered tenant that still owns rows cannot be deleted from
// the registry (FK violation 23503).
func TestTenantRegistry_DeleteWithData(t *testing.T) {
	t.Parallel()
	assets, _, _ := genericRepos(t)
	pool := registryPool(t)
	ctx := context.Background()

	if err := ensureTenants(ctx, pool, tenantA); err != nil {
		t.Fatalf("ensureTenants: %v", err)
	}

	// Seed an asset under tenantA; it is cleaned up so the test is
	// re-runnable and leaves the shared schema unmodified.
	created, err := assets.Create(ctx, testAsset(), repo.Tenant(tenantA))
	if err != nil {
		t.Fatalf("Create asset under tenantA: %v", err)
	}
	t.Cleanup(func() {
		if err := assets.Delete(context.Background(), created.ID, repo.Tenant(tenantA)); err != nil {
			t.Errorf("cleanup delete asset: %v", err)
		}
	})

	_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantA)
	pgFKViolation(t, "DELETE tenantA with owned assets", err)
}

// TestTenantRegistry_Backfill verifies migration 00002's backfill (spec
// scenario "Existing data is backfilled into the registry"): every distinct
// pre-existing tenant_id owns a registry row BEFORE the FK constraints are
// enforced, and afterwards tenant-owned rows must reference a registered
// tenant.
//
// It runs a two-step goose migration in a private scratch schema (p_backfill)
// with a private-only search_path: UpTo 00001 (no registry, no FKs, no RLS),
// plain unbound inserts under a legacy tenant id, then UpTo 00002. The
// scratch schema is dropped and recreated at test start, which also makes the
// test idempotent across test binary invocations (the schema persists on the
// database).
func TestTenantRegistry_Backfill(t *testing.T) {
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	t.Parallel()

	const schema = "p_backfill"

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	// Private-ONLY search_path (no ",public" fallback). With the fallback,
	// goose resolves public.goose_db_version (migrated by the dev server) and
	// skips migrating the scratch schema entirely.
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	// Clean slate: the schema persists across test binary invocations.
	if err := poolExec(ctx, cfg, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
		t.Fatalf("drop scratch schema: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create scratch schema: %v", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()
	if err := goose.UpTo(stdDB, migrationsDir, 1); err != nil {
		t.Fatalf("migrate to 00001: %v", err)
	}

	// Legacy data predates the registry: RLS (00003) and the FKs (00002) do
	// not exist yet at version 1, so plain unbound inserts are legal.
	const legacyTenant = "legacy-tenant-x"
	if _, err := pool.Exec(ctx,
		`INSERT INTO assets (tenant_id, doc_type, norm_serial) VALUES ($1, $2, $3)`,
		legacyTenant, "invoice", "backfill-legacy-1"); err != nil {
		t.Fatalf("seed legacy asset: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO sources (tenant_id, filename, content_type, byte_size, storage_path, sha256) VALUES ($1, $2, $3, $4, $5, $6)`,
		legacyTenant, "legacy.pdf", "application/pdf", 1234,
		"storage/legacy.pdf", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"); err != nil {
		t.Fatalf("seed legacy source: %v", err)
	}

	if err := goose.UpTo(stdDB, migrationsDir, 2); err != nil {
		t.Fatalf("migrate to 00002: %v", err)
	}

	// (a) Backfill ran: the legacy tenant id has a registry row.
	var registered bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)`, legacyTenant).Scan(&registered); err != nil {
		t.Fatalf("check registry row: %v", err)
	}
	if !registered {
		t.Error("backfill: legacy tenant id has no registry row")
	}

	// (b) FK enforcement is now active: RLS is not enabled in this scratch
	// schema (00003 was not run), so this unbound insert fails only if the
	// registry FK constraint fires.
	_, err = pool.Exec(ctx,
		`INSERT INTO assets (tenant_id, doc_type) VALUES ('not-in-registry-y', 'other')`)
	pgFKViolation(t, "INSERT asset for unregistered tenant", err)
}

// poolExec runs a single statement with a short-lived pool built from cfg,
// for setup work (schema drop) that must happen before the long-lived test
// pool exists.
func poolExec(ctx context.Context, cfg *pgxpool.Config, query string) error {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, query)
	return err
}
