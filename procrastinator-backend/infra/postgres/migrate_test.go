package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const migrateSchema = "p_migrate"

// TestMigrate verifies that migration 00006 (asset-management-rework) applies
// UP cleanly on top of 00001-00005, reverses cleanly back to version 5, and is
// repeatable (UP again). It asserts the observable schema shape before and
// after each phase rather than trusting the SQL by eye.
func TestMigrate(t *testing.T) {
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	pool, err := openTestPool(t, migrateSchema)
	if err != nil {
		t.Fatalf("openTestPool(p_migrate): %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// --- Phase 1: post-UP schema assertions (00001..00006 applied). ---
	for _, col := range []string{
		"name", "norm_name", "asset_category", "category_confidence",
		"category_user_set", "deleted_at", "merged_into", "merged_at",
	} {
		assertColumnExists(t, ctx, pool, "assets", col, true)
	}
	assertColumnExists(t, ctx, pool, "assets", "doc_type", false)

	if nullable := columnNullable(t, ctx, pool, "documents", "asset_id"); nullable != "YES" {
		t.Errorf("documents.asset_id is_nullable = %q, want \"YES\"", nullable)
	}

	if dt := columnDataType(t, ctx, pool, "ingest_reviews", "provenance"); dt != "jsonb" {
		t.Errorf("ingest_reviews.provenance data_type = %q, want \"jsonb\"", dt)
	}

	assertDocTypeCheckExpanded(t, ctx, pool, "documents", "documents_doc_type_check")
	assertDocTypeCheckExpanded(t, ctx, pool, "ingest_reviews", "ingest_reviews_doc_type_check")

	assertIndexExists(t, ctx, pool, "idx_assets_owner_norm_brand_model")
	assertIndexExists(t, ctx, pool, "idx_assets_owner_norm_name_model")

	// --- Phase 2: apply DOWN to version 5 (reverses 00006). ---
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose.SetDialect: %v", err)
	}
	goose.SetTableName(migrateSchema + ".goose_db_version")
	if err := goose.DownTo(stdDB, migrationsDir, 5); err != nil {
		t.Fatalf("goose.DownTo(5): %v", err)
	}

	// --- Phase 3: post-DOWN schema assertions. ---
	assertColumnExists(t, ctx, pool, "assets", "doc_type", true)
	for _, col := range []string{"name", "asset_category", "deleted_at", "merged_into"} {
		assertColumnExists(t, ctx, pool, "assets", col, false)
	}
	if nullable := columnNullable(t, ctx, pool, "documents", "asset_id"); nullable != "NO" {
		t.Errorf("documents.asset_id is_nullable = %q, want \"NO\"", nullable)
	}
	assertColumnExists(t, ctx, pool, "ingest_reviews", "provenance", false)

	assertDocTypeCheckRestored(t, ctx, pool, "documents", "documents_doc_type_check")

	// --- Phase 4: re-apply UP to prove repeatability. ---
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		t.Fatalf("goose.Up (re-apply): %v", err)
	}
	assertColumnExists(t, ctx, pool, "assets", "name", true)
	assertColumnExists(t, ctx, pool, "assets", "asset_category", true)
}

// assertColumnExists fails the test if the table's column presence does not match want.
func assertColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string, want bool) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`,
		migrateSchema, table, column).Scan(&count)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if got := count > 0; got != want {
		wantStr := "have"
		if !want {
			wantStr = "not have"
		}
		t.Errorf("table %q should %s column %q (found %d rows)", table, wantStr, column, count)
	}
}

// columnNullable returns the information_schema is_nullable value for a column.
func columnNullable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) string {
	t.Helper()
	var nullable string
	err := pool.QueryRow(ctx,
		`SELECT is_nullable FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`,
		migrateSchema, table, column).Scan(&nullable)
	if err != nil {
		t.Fatalf("is_nullable for %s.%s: %v", table, column, err)
	}
	return nullable
}

// columnDataType returns the information_schema data_type for a column.
func columnDataType(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) string {
	t.Helper()
	var dt string
	err := pool.QueryRow(ctx,
		`SELECT data_type FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`,
		migrateSchema, table, column).Scan(&dt)
	if err != nil {
		t.Fatalf("data_type for %s.%s: %v", table, column, err)
	}
	return dt
}

// fetchDocTypeCheckDef returns pg_get_constraintdef for the named check constraint.
func fetchDocTypeCheckDef(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, constraint string) string {
	t.Helper()
	var def string
	err := pool.QueryRow(ctx,
		`SELECT pg_get_constraintdef(c.oid)
		 FROM pg_constraint c
		 JOIN pg_class rel ON rel.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = rel.relnamespace
		 WHERE n.nspname = $1 AND rel.relname = $2 AND c.conname = $3`,
		migrateSchema, table, constraint).Scan(&def)
	if err != nil {
		t.Fatalf("constraint def %s.%s: %v", table, constraint, err)
	}
	return def
}

// assertDocTypeCheckExpanded asserts the check contains receipt AND statement (6-value set).
func assertDocTypeCheckExpanded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, constraint string) {
	t.Helper()
	def := fetchDocTypeCheckDef(t, ctx, pool, table, constraint)
	if !strings.Contains(def, "'receipt'") || !strings.Contains(def, "'statement'") {
		t.Errorf("%s.%s check = %q, want it to contain 'receipt' and 'statement'", table, constraint, def)
	}
}

// assertDocTypeCheckRestored asserts the check is back to the 4-value set:
// contains invoice but NOT statement.
func assertDocTypeCheckRestored(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, constraint string) {
	t.Helper()
	def := fetchDocTypeCheckDef(t, ctx, pool, table, constraint)
	if !strings.Contains(def, "'invoice'") {
		t.Errorf("%s.%s check = %q, want it to contain 'invoice'", table, constraint, def)
	}
	if strings.Contains(def, "'statement'") {
		t.Errorf("%s.%s check = %q, should NOT contain 'statement' (expected 4-value set)", table, constraint, def)
	}
}

// assertIndexExists fails the test if the named index is not present in the schema.
func assertIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE schemaname = $1 AND indexname = $2`,
		migrateSchema, index).Scan(&count)
	if err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	if count == 0 {
		t.Errorf("index %q not found in schema %q", index, migrateSchema)
	}
}
