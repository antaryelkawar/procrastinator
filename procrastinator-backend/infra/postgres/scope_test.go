package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

const testSchemaScope = "p_scope"

var (
	scopeOnce       sync.Once
	scopePool       *pgxpool.Pool
	scopeAssets     *postgres.AssetRepository
	scopeHouseholds *postgres.HouseholdRepository
	scopeInitErr    error
)

// scopeRepos returns the asset and household repositories bound to the p_scope
// test schema. Uses the sync.Once lazy init pattern (see generic_test.go): the
// pool and repositories are opened and initialized exactly once per test
// binary run.
func scopeRepos(t *testing.T) (*postgres.AssetRepository, *postgres.HouseholdRepository) {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	scopeOnce.Do(func() {
		scopePool, scopeInitErr = openTestPool(t, testSchemaScope)
		if scopeInitErr != nil {
			return
		}
		scopeAssets = postgres.NewAssetRepository(scopePool)
		scopeHouseholds = postgres.NewHouseholdRepository(scopePool)
	})
	if scopeInitErr != nil {
		t.Fatalf("init scope test pool: %v", scopeInitErr)
	}
	return scopeAssets, scopeHouseholds
}

// scopePoolFor returns the p_scope pool (for tenant registration via
// ensureTenants). It is lazily initialized inside scopeOnce alongside the
// repositories.
func scopePoolFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	scopeRepos(t) // ensure the pool is initialized
	return scopePool
}

// truncateScope clears all test data on the scope pool. Call at the top of
// each write test. The tenants table is NOT truncated: it is the user
// registry that households and memberships reference.
func truncateScope(t *testing.T) {
	t.Helper()
	scopeRepos(t) // ensure the pool is initialized
	if _, err := scopePool.Exec(context.Background(),
		`TRUNCATE household_members, households, assets, sources, documents CASCADE`); err != nil {
		t.Fatalf("truncate scope: %v", err)
	}
}

// TestHouseholdCRUD verifies the generic CRUD lifecycle for a household row:
// Create returns a DB-generated id and stamps the owning tenant; Get and List
// round-trip under the owning tenant; Delete removes the row (subsequent Get
// is ErrNotFound).
func TestHouseholdCRUD(t *testing.T) {
	_, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureTenants(ctx, scopePoolFor(t), "alice"); err != nil {
		t.Fatalf("ensureTenants: %v", err)
	}

	created, err := households.Create(ctx, entity.Household{DisplayName: "The Smiths"}, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID, want DB-generated id")
	}
	if created.TenantID != "alice" {
		t.Errorf("TenantID = %q, want alice", created.TenantID)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want DB default now()")
	}

	got, err := households.Get(ctx, created.ID, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "The Smiths" {
		t.Errorf("Get DisplayName = %q, want %q", got.DisplayName, "The Smiths")
	}

	list, err := households.List(ctx, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %d rows, want exactly 1 (got %v)", len(list), list)
	}

	if err := households.Delete(ctx, created.ID, repo.Tenant("alice")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := households.Get(ctx, created.ID, repo.Tenant("alice")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

// TestHouseholdMembership verifies the membership join table: AddMember is
// idempotent for a duplicate (household, user) pair, ListMembers returns the
// distinct members ordered by user id, and HouseholdsForUser returns the
// single shared household for every member.
func TestHouseholdMembership(t *testing.T) {
	_, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureTenants(ctx, scopePoolFor(t), "alice", "bob"); err != nil {
		t.Fatalf("ensureTenants: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "Shared"}, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}

	// Alice twice (idempotent) then bob.
	if err := households.AddMember(ctx, h.ID, "alice", repo.Tenant("alice")); err != nil {
		t.Fatalf("AddMember(alice): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "alice", repo.Tenant("alice")); err != nil {
		t.Fatalf("AddMember(alice) again (idempotent): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "bob", repo.Tenant("alice")); err != nil {
		t.Fatalf("AddMember(bob): %v", err)
	}

	members, err := households.ListMembers(ctx, h.ID, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("ListMembers = %d members, want exactly 2 (got %v)", len(members), members)
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m.UserID] = true
	}
	if !seen["alice"] || !seen["bob"] {
		t.Errorf("members = %v, want alice and bob", seen)
	}

	for _, user := range []string{"alice", "bob"} {
		ids, err := households.HouseholdsForUser(ctx, user, repo.Tenant("alice"))
		if err != nil {
			t.Fatalf("HouseholdsForUser(%s): %v", user, err)
		}
		if len(ids) != 1 || ids[0] != h.ID {
			t.Errorf("HouseholdsForUser(%s) = %v, want [%s]", user, ids, h.ID)
		}
	}
}

// TestScopeFilteredAssetList verifies the spec's scope access rule end to
// end: a user sees their own personal rows plus rows owned by a household
// they are a member of, and nothing else. Alice is a member of h1 only, so
// she sees A1 (her personal row) and A2 (h1's household row) but not A3 (h2's
// row, which she is not a member of) nor A4 (bob's personal row). Bob, with
// no households, sees only his own personal row A4.
func TestScopeFilteredAssetList(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureTenants(ctx, scopePoolFor(t), "alice", "bob"); err != nil {
		t.Fatalf("ensureTenants: %v", err)
	}

	h1, err := households.Create(ctx, entity.Household{DisplayName: "H1"}, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("Create h1: %v", err)
	}
	h2, err := households.Create(ctx, entity.Household{DisplayName: "H2"}, repo.Tenant("alice"))
	if err != nil {
		t.Fatalf("Create h2: %v", err)
	}

	// Alice is a member of h1 ONLY.
	if err := households.AddMember(ctx, h1.ID, "alice", repo.Tenant("alice")); err != nil {
		t.Fatalf("AddMember(alice -> h1): %v", err)
	}

	seed := func(t *testing.T, serial string, scopeType string, ownerHH *string, ownerTenant string) string {
		t.Helper()
		a := testAsset(func(a *entity.Asset) {
			a.SerialNumber = &serial
		})
		a.ScopeType = scopeType
		a.OwnerHouseholdID = ownerHH
		created, err := assets.Create(ctx, a, repo.Tenant(ownerTenant))
		if err != nil {
			t.Fatalf("Create asset %s: %v", serial, err)
		}
		return created.ID
	}

	a1 := seed(t, "SCOPE-A1", entity.ScopePersonal, nil, "alice")
	a2 := seed(t, "SCOPE-A2", entity.ScopeHousehold, &h1.ID, "alice")
	a3 := seed(t, "SCOPE-A3", entity.ScopeHousehold, &h2.ID, "alice")
	a4 := seed(t, "SCOPE-A4", entity.ScopePersonal, nil, "bob")

	check := func(t *testing.T, name string, tenant string, wantVisible, wantHidden map[string]bool) {
		t.Helper()
		got, err := assets.ListScoped(ctx, repo.Tenant(tenant))
		if err != nil {
			t.Fatalf("ListScoped(%s): %v", tenant, err)
		}
		ids := map[string]bool{}
		for _, a := range got {
			ids[a.ID] = true
		}
		for id := range wantVisible {
			if !ids[id] {
				t.Errorf("%s: asset %s should be visible but was not (got %v)", name, id, ids)
			}
		}
		for id := range wantHidden {
			if ids[id] {
				t.Errorf("%s: asset %s should NOT be visible but was (got %v)", name, id, ids)
			}
		}
	}

	check(t, "alice", "alice",
		map[string]bool{a1: true, a2: true},
		map[string]bool{a3: true, a4: true},
	)
	// Bonus: bob has no households -> only his own personal row is visible.
	check(t, "bob", "bob",
		map[string]bool{a4: true},
		map[string]bool{a1: true, a2: true, a3: true},
	)
}
