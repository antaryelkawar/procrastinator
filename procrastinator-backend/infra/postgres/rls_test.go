package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rls_test.go: raw-SQL integration tests for the row-level security
// (defense-in-depth) requirement from openspec
// owner-model-rework §"Row-level security
// defense-in-depth". These deliberately bypass the repository layer — RLS must
// hold even when application-level WHERE owner_id scoping is absent.
//
// Topology under test (empirically verified): the test database role is a
// NON-superuser without BYPASSRLS and OWNS every table, and migration
// 00004 has ENABLEd and FORCEd RLS on assets/sources/documents with a policy
// keyed on owner_id = current_setting('app.user_id', true). An unbound
// transaction therefore sees nothing, and cross-user writes are rejected
// with SQLSTATE 42501.

const testSchemaRLS = "p_rls"

const (
	rlsUserAcme   = "acme"
	rlsUserGlobex = "globex"

	seedAcmeSerial   = "rls-acme-1"
	seedGlobexSerial = "rls-globex-1"
)

var (
	rlsOnce    sync.Once
	rlsPool    *pgxpool.Pool
	rlsInitErr error
)

// rlsSharedPool returns the shared pool bound to the p_rls test schema (the
// package-level rlsPool variable), lazily initialized (sync.Once) with the two
// RLS seed rows. It mirrors the genericRepos lazy-init pattern.
func rlsSharedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	rlsOnce.Do(func() {
		rlsPool, rlsInitErr = openTestPool(t, testSchemaRLS)
		if rlsInitErr != nil {
			return
		}
		rlsInitErr = seedRLSFixture(context.Background(), rlsPool)
	})
	if rlsInitErr != nil {
		t.Fatalf("init rls test pool: %v", rlsInitErr)
	}
	return rlsPool
}

// seedRLSFixture registers the two RLS users and seeds exactly one asset per
// user, each inside a user-bound transaction (raw unbound writes are
// rejected by RLS, so seeding must bind the user first).
func seedRLSFixture(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureUsers(ctx, pool, rlsUserAcme, rlsUserGlobex); err != nil {
		return fmt.Errorf("ensureUsers: %w", err)
	}
	if err := seedAssetInBoundTx(ctx, pool, rlsUserAcme, "A", seedAcmeSerial); err != nil {
		return err
	}
	return seedAssetInBoundTx(ctx, pool, rlsUserGlobex, "G", seedGlobexSerial)
}

// seedAssetInBoundTx inserts a single asset row inside a transaction bound to
// the given user via a transaction-scoped set_config. It first deletes any
// existing row with the same marker so the fixture resets idempotently: the
// p_rls schema persists across separate test binary invocations, so a plain
// INSERT would hit the (owner_id, owner_household_id, norm_serial) unique
// index on re-runs.
func seedAssetInBoundTx(ctx context.Context, pool *pgxpool.Pool, userID, brand, serial string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed tx for %q: %w", userID, err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		return fmt.Errorf("bind seed user %q: %w", userID, err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM assets WHERE owner_id = $1 AND (payload #>> '{data,norm_serial}') = $2`,
		userID, serial); err != nil {
		return fmt.Errorf("reset seed asset for %q: %w", userID, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO assets (owner_id, payload) VALUES ($1, jsonb_build_object('data', jsonb_build_object('brand', $2::text, 'norm_serial', $3::text)))`,
		userID, brand, serial); err != nil {
		return fmt.Errorf("seed asset for %q: %w", userID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seed tx for %q: %w", userID, err)
	}
	return nil
}

// assertRLSViolation reports whether err is a PostgreSQL row-level security
// violation (SQLSTATE 42501).
func assertRLSViolation(t *testing.T, op string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: succeeded, want RLS violation (42501)", op)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("%s: err = %v, want row-level security violation (42501)", op, err)
	}
}

// bindUser opens a tx on pool and binds the given user via a
// transaction-scoped set_config. Returns the tx; caller owns Commit/Rollback.
func bindUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("bind user %q: %v", userID, err)
	}
	return tx
}

// TestRLS_BoundTransactionSeesOnlyBoundUser maps to the spec scenario
// "Unscoped query returns only the bound user's rows": a transaction bound to
// acme running a raw SELECT with no WHERE clause sees only acme rows.
func TestRLS_BoundTransactionSeesOnlyBoundUser(t *testing.T) {
	t.Parallel()
	pool := rlsSharedPool(t)
	ctx := context.Background()

	tx := bindUser(t, ctx, pool, rlsUserAcme)
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT owner_id FROM assets`)
	if err != nil {
		t.Fatalf("unscoped select: %v", err)
	}
	var seen []string
	for rows.Next() {
		var ownerID string
		if err := rows.Scan(&ownerID); err != nil {
			rows.Close()
			t.Fatalf("scan owner_id: %v", err)
		}
		seen = append(seen, ownerID)
		if ownerID != rlsUserAcme {
			t.Errorf("unscoped select returned user %q, want only %q", ownerID, rlsUserAcme)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate rows: %v", err)
	}
	rows.Close()

	if len(seen) == 0 {
		t.Fatalf("unscoped select returned 0 rows, want at least the acme seed row")
	}

	// The seed row must be visible to the acme-bound transaction.
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE (payload #>> '{data,norm_serial}') = $1`, seedAcmeSerial).Scan(&n); err != nil {
		t.Fatalf("count seed acme: %v", err)
	}
	if n != 1 {
		t.Errorf("seed row %q visible %d times, want 1", seedAcmeSerial, n)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestRLS_CrossUserWriteRejectedByPolicy maps to the spec scenario
// "Cross-user write is rejected by policy": a transaction bound to acme
// cannot create or modify a globex row.
func TestRLS_CrossUserWriteRejectedByPolicy(t *testing.T) {
	t.Parallel()
	pool := rlsSharedPool(t)
	ctx := context.Background()

	t.Run("insert into foreign user rejected", func(t *testing.T) {
		tx := bindUser(t, ctx, pool, rlsUserAcme)
		defer tx.Rollback(ctx)

		_, err := tx.Exec(ctx,
			`INSERT INTO assets (owner_id, payload) VALUES ('globex', jsonb_build_object('data', jsonb_build_object('brand', 'X', 'norm_serial', 'rls-x-1')))`)
		assertRLSViolation(t, "INSERT globex row while bound to acme", err)
		tx.Rollback(ctx)

		// The failed insert created nothing (verify from the globex side).
		txG := bindUser(t, ctx, pool, rlsUserGlobex)
		defer txG.Rollback(ctx)
		var n int
		if err := txG.QueryRow(ctx, `SELECT count(*) FROM assets WHERE (payload #>> '{data,norm_serial}') = 'rls-x-1'`).Scan(&n); err != nil {
			t.Fatalf("count failed-insert row: %v", err)
		}
		if n != 0 {
			t.Errorf("failed insert created %d rows, want 0", n)
		}
		txG.Rollback(ctx)
	})

	t.Run("update other user rows is a no-op", func(t *testing.T) {
		tx := bindUser(t, ctx, pool, rlsUserAcme)
		defer tx.Rollback(ctx)

		// globex rows are invisible to acme via the USING policy → 0 rows.
		tag, err := tx.Exec(ctx, `UPDATE assets SET payload = jsonb_set(payload, '{data,brand}', '"hacked"') WHERE owner_id = 'globex'`)
		if err != nil {
			t.Fatalf("UPDATE other-user rows: %v", err)
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("UPDATE other-user rows affected %d, want 0", tag.RowsAffected())
		}
		tx.Rollback(ctx)

		// The globex seed row is unmodified.
		txG := bindUser(t, ctx, pool, rlsUserGlobex)
		defer txG.Rollback(ctx)
		var brand string
		if err := txG.QueryRow(ctx,
			`SELECT payload #>> '{data,brand}' FROM assets WHERE (payload #>> '{data,norm_serial}') = $1`, seedGlobexSerial).Scan(&brand); err != nil {
			t.Fatalf("read globex seed brand: %v", err)
		}
		if brand != "G" {
			t.Errorf("globex seed brand = %q, want unchanged G", brand)
		}
		txG.Rollback(ctx)
	})

	t.Run("repoint own row to foreign user rejected", func(t *testing.T) {
		tx := bindUser(t, ctx, pool, rlsUserAcme)
		defer tx.Rollback(ctx)

		_, err := tx.Exec(ctx, `UPDATE assets SET owner_id = 'globex' WHERE owner_id = 'acme'`)
		assertRLSViolation(t, "UPDATE acme rows to globex user (WITH CHECK)", err)
		tx.Rollback(ctx)

		// The acme seed row still belongs to acme.
		txA := bindUser(t, ctx, pool, rlsUserAcme)
		defer txA.Rollback(ctx)
		var ownerID string
		if err := txA.QueryRow(ctx,
			`SELECT owner_id FROM assets WHERE (payload #>> '{data,norm_serial}') = $1`, seedAcmeSerial).Scan(&ownerID); err != nil {
			t.Fatalf("read acme seed owner: %v", err)
		}
		if ownerID != rlsUserAcme {
			t.Errorf("acme seed owner_id = %q, want acme", ownerID)
		}
		txA.Rollback(ctx)
	})
}

// TestRLS_UnboundTransactionSeesNothing maps to the spec scenario
// "Unbound transaction sees nothing": a transaction that never bound a user
// sees zero rows and cannot write any.
func TestRLS_UnboundTransactionSeesNothing(t *testing.T) {
	t.Parallel()
	pool := rlsSharedPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unbound tx: %v", err)
	}
	defer tx.Rollback(ctx)

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&count); err != nil {
		t.Fatalf("unbound count: %v", err)
	}
	if count != 0 {
		t.Errorf("unbound SELECT count = %d, want 0", count)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO assets (owner_id, payload) VALUES ('acme', jsonb_build_object('data', jsonb_build_object('brand', 'X', 'norm_serial', 'rls-unbound-1')))`)
	assertRLSViolation(t, "INSERT while unbound", err)
	tx.Rollback(ctx)
}

// TestRLS_UserBindingDoesNotLeakAcrossPoolConnections maps to the spec
// scenario "User binding does not leak across pooled connections": a
// transaction bound to acme commits, the same physical connection is reused
// for a transaction bound to globex, which then observes only globex rows with
// no residue of acme's binding. A MaxConns:1 pool guarantees the three txs
// land on the same physical connection.
func TestRLS_UserBindingDoesNotLeakAcrossPoolConnections(t *testing.T) {
	t.Parallel()
	rlsSharedPool(t) // ensure schema, migrations, and seed rows exist first

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dedicated pool config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = testSchemaRLS + ",public"
	cfg.MaxConns = 1

	dedicated, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("new dedicated pool: %v", err)
	}
	defer dedicated.Close()

	marker := fmt.Sprintf("rls-reuse-%d", time.Now().UnixNano())

	// Tx A: bound to acme, commits a row with the per-run marker.
	{
		tx := bindUser(t, ctx, dedicated, rlsUserAcme)
		if _, err := tx.Exec(ctx,
			`INSERT INTO assets (owner_id, payload) VALUES ($1, jsonb_build_object('data', jsonb_build_object('brand', $2::text, 'norm_serial', $3::text)))`,
			rlsUserAcme, "R", marker); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("tx A insert: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("tx A commit: %v", err)
		}
	}

	// Tx B: bound to globex on the same connection; must see only globex rows.
	{
		tx := bindUser(t, ctx, dedicated, rlsUserGlobex)
		rows, err := tx.Query(ctx, `SELECT owner_id FROM assets`)
		if err != nil {
			tx.Rollback(ctx)
			t.Fatalf("tx B unscoped select: %v", err)
		}
		seen := 0
		for rows.Next() {
			var ownerID string
			if err := rows.Scan(&ownerID); err != nil {
				rows.Close()
				tx.Rollback(ctx)
				t.Fatalf("tx B scan: %v", err)
			}
			seen++
			if ownerID != rlsUserGlobex {
				t.Errorf("tx B saw user %q, want only %q (no acme residue)", ownerID, rlsUserGlobex)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			tx.Rollback(ctx)
			t.Fatalf("tx B iterate: %v", err)
		}
		rows.Close()
		if seen == 0 {
			t.Errorf("tx B saw 0 rows, want at least the globex seed row")
		}
		tx.Commit(ctx)
	}

	// Tx C: no binding at all — the discriminating residue check. A
	// session-scoped binding would leak rows here; a transaction-scoped one
	// must see nothing.
	{
		tx, err := dedicated.Begin(ctx)
		if err != nil {
			t.Fatalf("tx C begin: %v", err)
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&count); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("tx C unbound count: %v", err)
		}
		if count != 0 {
			t.Errorf("tx C unbound count = %d, want 0 (no binding residue)", count)
		}
		tx.Rollback(ctx)
	}

	// Cleanup: delete the per-run row, bound to acme.
	{
		tx := bindUser(t, ctx, dedicated, rlsUserAcme)
		tag, err := tx.Exec(ctx, `DELETE FROM assets WHERE owner_id = $1 AND (payload #>> '{data,norm_serial}') = $2`, rlsUserAcme, marker)
		if err != nil {
			tx.Rollback(ctx)
			t.Fatalf("cleanup delete: %v", err)
		}
		if tag.RowsAffected() != 1 {
			t.Errorf("cleanup delete affected %d, want 1", tag.RowsAffected())
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("cleanup commit: %v", err)
		}
	}
}

// TestRLS_HouseholdOwnerCanInsertWithoutMembershipRow ensures that a household
// owner can insert a household-scoped row even if they do not have a
// membership row for that household.
func TestRLS_HouseholdOwnerCanInsertWithoutMembershipRow(t *testing.T) {
	t.Parallel()
	pool := rlsSharedPool(t)
	ctx := context.Background()

	// 1. Create a household with rlsUserAcme as owner.
	// We bind to Acme to create the household.
	tx := bindUser(t, ctx, pool, rlsUserAcme)
	householdID := fmt.Sprintf("rls-hh-%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx,
		`INSERT INTO households (id, owner_id, payload)
		 VALUES ($1, $2, '{"data": {"display_name": "Acme Household"}}'::jsonb)`,
		householdID, rlsUserAcme); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("create household: %v", err)
	}

	// 2. Verify Acme has no membership row for this household.
	var memberCount int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM household_members WHERE household_id = $1 AND user_id = $2`,
		householdID, rlsUserAcme).Scan(&memberCount)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("count membership: %v", err)
	}
	if memberCount != 0 {
		t.Fatalf("expected 0 membership rows for owner, found %d", memberCount)
	}

	// 3. Try to insert an asset into this household.
	// This should pass because the owner is exempt from the membership check.
	_, err = tx.Exec(ctx,
		`INSERT INTO assets (owner_id, owner_household_id, payload)
		 VALUES ($1, $2, '{"data": {"brand": "A", "norm_serial": "rls-hh-1"}}'::jsonb)`,
		rlsUserAcme, householdID)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to insert asset as household owner: %v", err)
	}

	// 4. Verify insertion succeeded.
	var assetCount int
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM assets WHERE owner_household_id = $1`,
		householdID).Scan(&assetCount)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("count assets: %v", err)
	}
	if assetCount != 1 {
		t.Errorf("expected 1 asset, found %d", assetCount)
	}

	tx.Commit(ctx)
}
