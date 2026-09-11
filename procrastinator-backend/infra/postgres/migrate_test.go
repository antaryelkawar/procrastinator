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

// The uniform physical shape: every entity table carries key/FK/RLS/lifecycle
// columns plus created_at + updated_at + payload jsonb. household_members is a
// pure association table (the real FK pair is the tenancy predicate key) and is
// exempt from the payload shape.
var (
	// entityTables all carry `payload jsonb NOT NULL DEFAULT '{}'`.
	entityTables = []string{
		"users", "households",
		"sources", "assets", "documents",
		"financial_accounts", "import_batches", "money_movements", "import_lines",
		"ingest_reviews",
	}
	// allTables is the full set created by the fresh migration.
	allTables = append(append([]string{}, entityTables...), "household_members")
)

// TestMigrate verifies the fresh uniform-jsonb migration (00001_schema) applies
// UP cleanly, asserts the observable schema shape, reverses cleanly to version 0
// (empty), and is repeatable (UP again). It asserts the schema by querying the
// catalog rather than trusting the SQL by eye.
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

	// --- Phase 1: post-UP schema assertions. ---

	// 1a. All 11 tables exist.
	for _, tbl := range allTables {
		assertTableExists(t, ctx, pool, tbl, true)
	}

	// 1b. Every entity table carries the uniform payload column (jsonb, NOT
	// NULL, default '{}'). The association table does not.
	for _, tbl := range entityTables {
		assertPayloadColumn(t, ctx, pool, tbl)
	}
	assertColumnExists(t, ctx, pool, "household_members", "payload", false)

	// 1c. The legacy column-based shape is gone. A representative set of the
	// identity/data columns that moved into payload.data.* must no longer exist
	// as real columns.
	legacyColumns := map[string][]string{
		"assets": {
			"name", "brand", "model", "serial_number",
			"norm_serial", "norm_brand", "norm_model", "norm_name",
			"doc_type", "asset_category", "price", "currency",
			"purchase_date", "warranty_end", "metadata",
			"category_confidence", "category_user_set", "merged_into", "merged_at",
		},
		"sources": {
			"filename", "content_type", "size", "path", "sha256", "uploaded_at",
		},
		"documents": {
			"doc_type", "status", "filename",
		},
		"financial_accounts": {
			"name", "account_type", "institution", "balance",
		},
		"ingest_reviews": {
			"state", "doc_type", "provenance", "confidence",
		},
		"money_movements": {
			"amount", "description", "external_reference", "occurred_at",
		},
	}
	for tbl, cols := range legacyColumns {
		for _, col := range cols {
			assertColumnExists(t, ctx, pool, tbl, col, false)
		}
	}

	// 1d. documents.asset_id is nullable (purge detaches rather than cascades)
	// and its FK uses ON DELETE SET NULL.
	if nullable := columnNullable(t, ctx, pool, "documents", "asset_id"); nullable != "YES" {
		t.Errorf("documents.asset_id is_nullable = %q, want \"YES\"", nullable)
	}
	assertFKSetNull(t, ctx, pool, "documents", "asset_id")

	// 1e. documents.source_id is unique (at most one document per source).
	assertUniqueNonPK(t, ctx, pool, "documents")

	// 1f. The norm-serial uniqueness index: a partial UNIQUE index whose
	// definition references the COALESCE household key, the payload
	// norm_serial expression, and the deleted_at IS NULL predicate.
	def := indexDef(t, ctx, pool, "uniq_assets_owner_norm_serial")
	for _, want := range []string{"COALESCE", "norm_serial", "deleted_at IS NULL"} {
		if !strings.Contains(def, want) {
			t.Errorf("uniq_assets_owner_norm_serial def = %q, want it to contain %q", def, want)
		}
	}

	// 1g. The stage-1 resolver lookup index (non-unique expression composite)
	// exists alongside the unique index.
	assertIndexExists(t, ctx, pool, "idx_assets_owner_norm_serial")

	// 1h. The D2 jsonb-expression / trigram index list is present.
	for _, idx := range []string{
		"idx_assets_owner_norm_brand_model",
		"idx_assets_owner_norm_name_model",
		"idx_assets_data_name_trgm",
		"idx_assets_data_brand_trgm",
		"idx_assets_data_model_trgm",
		"idx_assets_data_serial_trgm",
		"idx_accounts_data_name_trgm",
		"idx_accounts_data_type_trgm",
		"idx_accounts_data_institution_trgm",
		"idx_movements_data_description_trgm",
		"idx_movements_data_reference_trgm",
		"idx_sources_data_filename_trgm",
		"idx_sources_owner_sha256",
		"idx_documents_owner_status",
		"idx_import_batches_owner_state",
		"idx_ingest_reviews_owner_state",
	} {
		assertIndexExists(t, ctx, pool, idx)
	}

	// 1i. pg_trgm is enabled (backs the trigram GIN indexes).
	assertExtensionPresent(t, ctx, pool, "pg_trgm")

	// --- Phase 2: apply DOWN to version 0 (reverses the whole fresh schema). ---
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose.SetDialect: %v", err)
	}
	goose.SetTableName(migrateSchema + ".goose_db_version")
	if err := goose.DownTo(stdDB, migrationsDir, 0); err != nil {
		t.Fatalf("goose.DownTo(0): %v", err)
	}

	// --- Phase 3: post-DOWN schema assertions. ---
	// All 11 tables are gone (entity tables and the association table).
	for _, tbl := range allTables {
		assertTableExists(t, ctx, pool, tbl, false)
	}
	// pg_trgm is intentionally NOT dropped (cluster-level object).
	assertExtensionPresent(t, ctx, pool, "pg_trgm")

	// --- Phase 4: re-apply UP to prove repeatability. ---
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		t.Fatalf("goose.Up (re-apply): %v", err)
	}
	for _, tbl := range allTables {
		assertTableExists(t, ctx, pool, tbl, true)
	}
	for _, tbl := range entityTables {
		assertPayloadColumn(t, ctx, pool, tbl)
	}
}

// assertTableExists fails the test if the table's presence does not match want.
func assertTableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, want bool) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_class c
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r','p')`,
		migrateSchema, table).Scan(&count)
	if err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	if got := count > 0; got != want {
		wantStr := "be present"
		if !want {
			wantStr = "be absent"
		}
		t.Errorf("table %q should %s in schema %q (found %d rows)", table, wantStr, migrateSchema, count)
	}
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

// assertPayloadColumn asserts the table carries a `payload` column that is
// jsonb, NOT NULL, and defaults to '{}'.
func assertPayloadColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) {
	t.Helper()
	assertColumnExists(t, ctx, pool, table, "payload", true)
	if dt := columnDataType(t, ctx, pool, table, "payload"); dt != "jsonb" {
		t.Errorf("%s.payload data_type = %q, want \"jsonb\"", table, dt)
	}
	if nullable := columnNullable(t, ctx, pool, table, "payload"); nullable != "NO" {
		t.Errorf("%s.payload is_nullable = %q, want \"NO\"", table, nullable)
	}
	var def string
	err := pool.QueryRow(ctx,
		`SELECT column_default FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = 'payload'`,
		migrateSchema, table).Scan(&def)
	if err != nil {
		t.Fatalf("column_default for %s.payload: %v", table, err)
	}
	if !strings.Contains(def, "'{}'") {
		t.Errorf("%s.payload column_default = %q, want it to default to '{}'", table, def)
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

// assertFKSetNull asserts the table.column carries a foreign key whose delete
// rule is ON DELETE SET NULL. It reads the authoritative pg_get_constraintdef
// (the same source \d uses) rather than the raw confdeltype catalog byte, which
// the human-readable definition renders reliably.
func assertFKSetNull(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	var def string
	err := pool.QueryRow(ctx,
		`SELECT pg_get_constraintdef(c.oid)
		 FROM pg_constraint c
		 JOIN pg_class rel ON rel.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = rel.relnamespace
		 WHERE n.nspname = $1 AND rel.relname = $2 AND c.contype = 'f'
		   AND pg_get_constraintdef(c.oid) LIKE ('FOREIGN KEY (' || $3 || ')%')`,
		migrateSchema, table, column).Scan(&def)
	if err != nil {
		t.Fatalf("FK definition for %s.%s: %v", table, column, err)
	}
	if !strings.Contains(def, "ON DELETE SET NULL") {
		t.Errorf("%s.%s FK def = %q, want it to contain \"ON DELETE SET NULL\"", table, column, def)
	}
}

// assertUniqueNonPK asserts the table has at least one non-primary-key unique
// index (i.e. a UNIQUE constraint distinct from the PK).
func assertUniqueNonPK(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM pg_index i
		 JOIN pg_class c ON c.oid = i.indrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relname = $2 AND i.indisunique AND NOT i.indisprimary`,
		migrateSchema, table).Scan(&count)
	if err != nil {
		t.Fatalf("check unique non-PK index on %s: %v", table, err)
	}
	if count == 0 {
		t.Errorf("table %q has no non-PK unique index (expected a UNIQUE constraint)", table)
	}
}

// indexDef returns pg_get_indexdef for the named index in the migrate schema.
// It is schema-scoped so a stale same-named index left in another per-test
// schema from a prior run is never picked up.
func indexDef(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) string {
	t.Helper()
	var def string
	err := pool.QueryRow(ctx,
		`SELECT pg_get_indexdef(i.indexrelid)
		 FROM pg_index i
		 JOIN pg_class c ON c.oid = i.indexrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relname = $2`,
		migrateSchema, index).Scan(&def)
	if err != nil {
		t.Fatalf("index def %s: %v", index, err)
	}
	return def
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

// assertExtensionPresent fails the test if the named extension is not installed.
func assertExtensionPresent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ext string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_extension WHERE extname = $1`,
		ext).Scan(&count)
	if err != nil {
		t.Fatalf("check extension %s: %v", ext, err)
	}
	if count == 0 {
		t.Errorf("extension %q not installed", ext)
	}
}
