package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

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

// TestUserRegistry_Has verifies the Has method:
// - true for the seeded test-user (migration 00002 seeds it)
// - false for an unknown user
// - true after inserting a registry row
func TestUserRegistry_Has(t *testing.T) {
	pool := registryPool(t)
	ctx := context.Background()
	registry := postgres.NewUserRegistry(pool)

	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"seeded user is registered", userA, true},
		{"unknown user is not registered", "unknown-user", false},
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

	t.Run("newly inserted user is registered", func(t *testing.T) {
		// ON CONFLICT keeps the test re-runnable: the shared p_generic
		// schema persists across runs, so the row may already exist.
		if _, err := pool.Exec(ctx, `INSERT INTO Users (id) VALUES ('new-user') ON CONFLICT (id) DO NOTHING`); err != nil {
			t.Fatalf("INSERT user: %v", err)
		}
		got, err := registry.Has(ctx, "new-user")
		if err != nil {
			t.Fatalf("Has(new-user): %v", err)
		}
		if !got {
			t.Error("Has(new-user) = false, want true")
		}
	})
}

// TestUserRegistry_FKEforcement verifies the registry's FK enforcement:
// user-owned rows must reference a registered user, so inserting an asset
// under an unregistered user fails with SQLSTATE 23503.
func TestUserRegistry_FKEforcement(t *testing.T) {
	t.Parallel()
	pool := registryPool(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, pool, userA); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	// The insert runs inside a user-bound transaction: with the
	// non-superuser app role (design D4) RLS actually applies, and the
	// policy's WITH CHECK passes only when the bound user equals the
	// row's owner_id. Binding to the unregistered id lets the FK check
	// (the registry enforcement under test) run and fail with 23503.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) // no-op on the expected error path
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, "unregistered-user"); err != nil {
		t.Fatalf("bind user: %v", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO assets (owner_id) VALUES ('unregistered-user')`)
	pgFKViolation(t, "INSERT asset for unregistered user", err)
}

// TestUserRegistry_DeleteWithData verifies the ON DELETE RESTRICT
// behavior: a registered user that still owns rows cannot be deleted from
// the registry (FK violation 23503).
func TestUserRegistry_DeleteWithData(t *testing.T) {
	// Not parallel: depends on the shared p_generic assets table not being
	// truncated by concurrent tests between Create and DELETE.
	assets, _, _ := genericRepos(t)
	pool := registryPool(t)
	ctx := context.Background()

	// Use a unique user to avoid lock contention with other parallel tests
	// that share the userA fixture.
	const uniqueUser = "delete-with-data-user"
	if err := ensureUsers(ctx, pool, uniqueUser); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	// Seed an asset under the unique user; it is cleaned up so the test is
	// re-runnable and leaves the shared schema unmodified.
	created, err := assets.Create(ctx, testAsset(), repo.Owner(uniqueUser))
	if err != nil {
		t.Fatalf("Create asset under unique user: %v", err)
	}
	t.Cleanup(func() {
		if err := assets.Delete(context.Background(), created.ID, repo.Owner(uniqueUser)); err != nil {
			t.Errorf("cleanup delete asset: %v", err)
		}
	})

	_, err = pool.Exec(ctx, `DELETE FROM Users WHERE id = $1`, uniqueUser)
	pgFKViolation(t, "DELETE user with owned assets", err)
}
