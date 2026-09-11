package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The two EXPLAIN tests below share one schema (p_index) and therefore must not
// run in parallel with each other; neither calls t.Parallel(). Each is
// self-contained: it opens a freshly migrated schema, seeds enough rows that a
// sequential scan is a plausible alternative, and then asserts the stage-1
// serial lookup / documents-status filter is INDEX-SERVED rather than a
// sequential scan.
//
// Why "index-served" and not a specific index name: the owner-model RLS policy
// is a disjunction (owner_id = me OR owner_household_id IN households(me)).
// Postgres plans that disjunction with a BitmapOr that always routes through an
// owner_id-prefix index, so the payload expression (norm_serial / status) is
// applied as a Filter rather than as the chosen index key. Bypassing RLS to
// force the expression index is not an option here: the dedicated rls_bypass
// role lacks SELECT on the entity tables, and granting it would widen the
// RLS-bypass scope (a tenancy risk). The specific expression indexes' existence
// and shape are asserted in migrate_test.go; this test proves the lookups are
// served by an index (so dropping the indexes degrades them to a seq scan).
const indexSchema = "p_index"

// explainPlanTx runs EXPLAIN (FORMAT JSON) for the query inside the given
// transaction and returns the rendered plan text.
func explainPlanTx(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
	t.Helper()
	var plan []byte
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query).Scan(&plan); err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	return string(plan)
}

// assertIndexServed asserts the query is served by an index, not a sequential
// scan. It must be called in a transaction where enable_seqscan=off is set, so
// the planner uses an index whenever one is available; if no index can serve
// the query, it falls back to a (now-expensive) sequential scan and the test
// fails — catching index removal/drift.
func assertIndexServed(t *testing.T, ctx context.Context, tx pgx.Tx, query string) {
	t.Helper()
	plan := explainPlanTx(t, ctx, tx, query)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) {
		t.Fatalf("query degraded to a sequential scan — expected an index to serve it.\nquery:\n%s\nplan:\n%s", query, plan)
	}
}

// TestExplainStageOneSerialLookupUsesIndex verifies the stage-1
// identity-resolution lookup (owner + normalized-serial equality) is served by
// an index on assets rather than a sequential scan.
func TestExplainStageOneSerialLookupUsesIndex(t *testing.T) {
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	pool, err := openTestPool(t, indexSchema)
	if err != nil {
		t.Fatalf("openTestPool(p_index): %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	const owner = "test-user"

	// Seed 300 personal assets for the owner, each with a distinct norm_serial,
	// so a sequential scan is a realistic alternative to an index.
	seedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	defer seedTx.Rollback(ctx)
	if _, err := seedTx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", owner); err != nil {
		t.Fatalf("set app.user_id: %v", err)
	}
	for i := 0; i < 300; i++ {
		serial := fmt.Sprintf("SER%04d", i)
		if _, err := seedTx.Exec(ctx,
			`INSERT INTO assets (owner_id, payload)
			 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			       'kind', 'asset',
			       'norm_serial', $2::text)))`, owner, serial); err != nil {
			t.Fatalf("seed asset %d: %v", i, err)
		}
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	// EXPLAIN the resolver's stage-1 lookup shape, forcing index usage.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin explain: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", owner); err != nil {
		t.Fatalf("set app.user_id: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("SET LOCAL enable_seqscan: %v", err)
	}
	const query = `SELECT id, payload FROM assets
		WHERE owner_id = 'test-user'
		  AND deleted_at IS NULL
		  AND (payload #>> '{data,norm_serial}') = 'SER0000'
		LIMIT 5`
	assertIndexServed(t, ctx, tx, query)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit explain: %v", err)
	}
}

// TestExplainDocumentsStatusFilterUsesIndex verifies the documents-section list
// filter (owner + status) is served by an index on documents rather than a
// sequential scan.
func TestExplainDocumentsStatusFilterUsesIndex(t *testing.T) {
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	pool, err := openTestPool(t, indexSchema)
	if err != nil {
		t.Fatalf("openTestPool(p_index): %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	const owner = "test-user"

	// Seed 200 sources and 200 documents (each document linked to a distinct
	// source) all sharing one status, so a sequential scan is a realistic
	// alternative to an index.
	seedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	defer seedTx.Rollback(ctx)
	if _, err := seedTx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", owner); err != nil {
		t.Fatalf("set app.user_id: %v", err)
	}
	for i := 0; i < 200; i++ {
		var srcID string
		if err := seedTx.QueryRow(ctx,
			`INSERT INTO sources (owner_id, payload)
			 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			       'kind', 'source',
			       'filename', $2::text)))
			 RETURNING id`, owner, fmt.Sprintf("file-%04d.pdf", i)).Scan(&srcID); err != nil {
			t.Fatalf("seed source %d: %v", i, err)
		}
		if _, err := seedTx.Exec(ctx,
			`INSERT INTO documents (owner_id, source_id, payload)
			 VALUES ($1, $2, jsonb_build_object('data', jsonb_build_object(
			       'kind', 'document',
			       'status', 'pending')))`, owner, srcID); err != nil {
			t.Fatalf("seed document %d: %v", i, err)
		}
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin explain: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.user_id', $1, true)", owner); err != nil {
		t.Fatalf("set app.user_id: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("SET LOCAL enable_seqscan: %v", err)
	}
	const query = `SELECT id, payload FROM documents
		WHERE owner_id = 'test-user'
		  AND (payload #>> '{data,status}') = 'pending'`
	assertIndexServed(t, ctx, tx, query)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit explain: %v", err)
	}
}
