package store_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"procrastinator-backend/internal/store"
)

// testSchema is a private schema owned by this test package. Each DB test
// package gets its own private schema because `go test` runs package test
// binaries concurrently (with default -p) and they share one database — a
// shared public schema would make per-test TRUNCATE isolation race across
// packages.
const testSchema = "lm_store"

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
	return st
}

func migrationsDir() string {
	return filepath.Join("..", "..", "migrations")
}

func queryInt(t *testing.T, st *store.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool().QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	return n
}

func TestMigrate(t *testing.T) {
	cases := []struct {
		name       string
		dropFirst  bool
		checkIndex bool
	}{
		{"creates tables and partial index on empty database", true, true},
		{"is idempotent", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			ctx := context.Background()

			if tc.dropFirst {
				// The goose version table (goose_db_version) must go too: with it
				// present, goose considers migration 1 already applied and would
				// not recreate the dropped tables.
				if _, err := st.Pool().Exec(ctx, "DROP TABLE IF EXISTS goose_db_version, documents, assets, sources CASCADE"); err != nil {
					t.Fatalf("failed to drop tables: %v", err)
				}
			}

			if err := st.Migrate(ctx, migrationsDir()); err != nil {
				t.Fatalf("migration failed: %v", err)
			}

			for _, table := range []string{"sources", "assets", "documents"} {
				var exists bool
				q := "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = '" + testSchema + "' AND table_name = $1)"
				if err := st.Pool().QueryRow(ctx, q, table).Scan(&exists); err != nil || !exists {
					t.Fatalf("table %s does not exist", table)
				}
			}

			if tc.checkIndex {
				var exists bool
				idxQuery := fmt.Sprintf(`
					SELECT EXISTS (
						SELECT 1 FROM pg_indexes
						WHERE schemaname = '%s'
						AND tablename = 'assets'
						AND indexdef ILIKE '%%UNIQUE%%'
						AND indexdef ILIKE '%%norm_serial%%'
						AND indexdef ILIKE '%%IS NOT NULL%%'
					)`, testSchema)
				if err := st.Pool().QueryRow(ctx, idxQuery).Scan(&exists); err != nil || !exists {
					t.Fatal("partial unique index on assets(norm_serial) WHERE norm_serial IS NOT NULL not found")
				}
			}
		})
	}
}

func TestOpenFailsFastOnUnreachableDB(t *testing.T) {
	t.Parallel()

	dsn := "postgres://postgres:postgres@127.0.0.1:59999/doesnotexist?sslmode=disable&connect_timeout=2"
	ctx := context.Background()

	start := time.Now()
	_, err := store.Open(ctx, dsn)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error opening unreachable database, got nil")
	}
	if elapsed > 15*time.Second {
		t.Fatalf("Open took too long: %v, expected less than 15s", elapsed)
	}
}

func TestTruncateAll(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	_ = st.Migrate(ctx, migrationsDir())

	var sourceID, assetID string
	if err := st.Pool().QueryRow(ctx,
		"INSERT INTO sources (filename, content_type, byte_size, storage_path, sha256) VALUES ('f', 't', 1, 'p', 's') RETURNING id",
	).Scan(&sourceID); err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	if err := st.Pool().QueryRow(ctx, "INSERT INTO assets DEFAULT VALUES RETURNING id").Scan(&assetID); err != nil {
		t.Fatalf("failed to insert asset: %v", err)
	}
	if _, err := st.Pool().Exec(ctx,
		"INSERT INTO documents (asset_id, source_id, doc_type, extracted_fields, raw_extraction) VALUES ($1, $2, 'invoice', '{}'::jsonb, '{}'::jsonb)",
		assetID, sourceID,
	); err != nil {
		t.Fatalf("failed to insert document: %v", err)
	}

	if queryInt(t, st, "SELECT count(*) FROM sources") != 1 {
		t.Fatal("expected 1 source")
	}
	if queryInt(t, st, "SELECT count(*) FROM assets") != 1 {
		t.Fatal("expected 1 asset")
	}
	if queryInt(t, st, "SELECT count(*) FROM documents") != 1 {
		t.Fatal("expected 1 document")
	}

	if err := st.TruncateAll(ctx); err != nil {
		t.Fatalf("TruncateAll failed: %v", err)
	}

	if queryInt(t, st, "SELECT count(*) FROM sources") != 0 {
		t.Fatal("expected 0 sources")
	}
	if queryInt(t, st, "SELECT count(*) FROM assets") != 0 {
		t.Fatal("expected 0 assets")
	}
	if queryInt(t, st, "SELECT count(*) FROM documents") != 0 {
		t.Fatal("expected 0 documents")
	}
}
