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

// scopePoolFor returns the p_scope pool (for user registration via
// ensureUsers). It is lazily initialized inside scopeOnce alongside the
// repositories.
func scopePoolFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	scopeRepos(t) // ensure the pool is initialized
	return scopePool
}

// truncateScope clears all test data on the scope pool. Call at the top of
// each write test. The Users table is NOT truncated: it is the user
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
// Create returns a DB-generated id and stamps the owning user; Get and List
// round-trip under the owning user; Delete removes the row (subsequent Get
// is ErrNotFound).
func TestHouseholdCRUD(t *testing.T) {
	_, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "alice"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	created, err := households.Create(ctx, entity.Household{DisplayName: "The Smiths"}, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID, want DB-generated id")
	}
	if created.OwnerID != "alice" {
		t.Errorf("OwnerID = %q, want alice", created.OwnerID)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want DB default now()")
	}

	got, err := households.Get(ctx, created.ID, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "The Smiths" {
		t.Errorf("Get DisplayName = %q, want %q", got.DisplayName, "The Smiths")
	}

	list, err := households.List(ctx, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %d rows, want exactly 1 (got %v)", len(list), list)
	}

	if err := households.Delete(ctx, created.ID, repo.Owner("alice")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := households.Get(ctx, created.ID, repo.Owner("alice")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

// TestHouseholdMembership verifies the membership join table: AddMember is
// idempotent for a duplicate (household, user) pair, ListMembers returns the
// distinct members ordered by user id, and HouseholdsForUser returns the
// single shared household for every member. The owner (alice) adds bob even
// though alice is not yet a member — exercising the owner disjunct of the
// household_members RLS policy.
func TestHouseholdMembership(t *testing.T) {
	_, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "alice", "bob"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "Shared"}, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}

	// Alice twice (idempotent) then bob (added by the owner alice).
	if err := households.AddMember(ctx, h.ID, "alice", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(alice): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "alice", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(alice) again (idempotent): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "bob", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(bob): %v", err)
	}

	members, err := households.ListMembers(ctx, h.ID, repo.Owner("alice"))
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

	for _, usr := range []string{"alice", "bob"} {
		ids, err := households.HouseholdsForUser(ctx, usr, repo.Owner("alice"))
		if err != nil {
			t.Fatalf("HouseholdsForUser(%s): %v", usr, err)
		}
		if len(ids) != 1 || ids[0] != h.ID {
			t.Errorf("HouseholdsForUser(%s) = %v, want [%s]", usr, ids, h.ID)
		}
	}
}

// TestScopeFilteredAssetList verifies the spec's scope access rule end to
// end: a user sees their own personal rows plus rows owned by a household
// they are a member of, and nothing else. Alice is a member of h1 only, so
// she sees A1 (her personal row) and A2 (h1's household row) but not A3 (h2's
// row, which she is not a member of) nor A4 (bob's personal row). Bob, with
// no households, sees only his own personal row A4.
//
// A3 is a household row owned by h2, a household alice is NOT a member of.
// The membership trigger (migration 00004 enforce_household_membership)
// forbids a non-member from attaching a row to a household, so a dedicated
// h2 member (carol) creates A3. carol is a member only of h2, so carol sees
// A3 while alice does not — the visibility assertion is preserved.
func TestScopeFilteredAssetList(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "alice", "bob", "carol"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h1, err := households.Create(ctx, entity.Household{DisplayName: "H1"}, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create h1: %v", err)
	}
	h2, err := households.Create(ctx, entity.Household{DisplayName: "H2"}, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create h2: %v", err)
	}

	// Alice is a member of h1 ONLY (h2 membership is intentionally absent so
	// A3 stays hidden from her).
	if err := households.AddMember(ctx, h1.ID, "alice", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(alice -> h1): %v", err)
	}
	// Carol is a member of h2 ONLY, so she can create the h2 household row A3
	// and sees it; alice (not an h2 member) does not.
	if err := households.AddMember(ctx, h2.ID, "carol", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(carol -> h2): %v", err)
	}

	seed := func(t *testing.T, serial string, ownerHH *string, ownerUser string) string {
		t.Helper()
		a := testAsset(func(a *entity.Asset) {
			a.SerialNumber = &serial
		})
		a.OwnerHouseholdID = ownerHH
		created, err := assets.Create(ctx, a, repo.Owner(ownerUser))
		if err != nil {
			t.Fatalf("Create asset %s: %v", serial, err)
		}
		return created.ID
	}

	a1 := seed(t, "SCOPE-A1", nil, "alice")
	a2 := seed(t, "SCOPE-A2", &h1.ID, "alice")
	a3 := seed(t, "SCOPE-A3", &h2.ID, "carol")
	a4 := seed(t, "SCOPE-A4", nil, "bob")

	check := func(t *testing.T, name string, userID string, wantVisible, wantHidden map[string]bool) {
		t.Helper()
		got, err := assets.List(ctx, repo.Owner(userID))
		if err != nil {
			t.Fatalf("List(%s): %v", userID, err)
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

// TestScopeMemberGetsHouseholdRow verifies the scope rule for Get: a
// non-owner member of a household can see (Get) a household row owned by the
// household, and can also Get their own personal row; a non-member is denied
// (ErrNotFound).
func TestScopeMemberGetsHouseholdRow(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "owner", "member", "stranger"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "owner", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(owner): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "member", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(member): %v", err)
	}

	// Owner creates a household-scoped asset (OwnerID=owner, OwnerHouseholdID=H).
	householdAsset := testAsset()
	householdAsset.OwnerHouseholdID = &h.ID
	created, err := assets.Create(ctx, householdAsset, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household asset: %v", err)
	}

	// The member (NOT the owner) can Get the household row.
	got, err := assets.Get(ctx, created.ID, repo.Owner("member"))
	if err != nil {
		t.Fatalf("member Get household row: %v (want success)", err)
	}
	if got.ID != created.ID {
		t.Fatalf("member Get household row ID = %q, want %q", got.ID, created.ID)
	}

	// The member can also Get their OWN personal row.
	personal := testAsset(func(a *entity.Asset) {
		a.SerialNumber = strPtr("SCOPE-PERSONAL-1")
	})
	personalID, err := assets.Create(ctx, personal, repo.Owner("member"))
	if err != nil {
		t.Fatalf("Create member personal asset: %v", err)
	}
	if _, err := assets.Get(ctx, personalID.ID, repo.Owner("member")); err != nil {
		t.Fatalf("member Get own personal row: %v (want success)", err)
	}

	// A non-member cannot Get the household row.
	if _, err := assets.Get(ctx, created.ID, repo.Owner("stranger")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("stranger Get household row: err = %v, want ErrNotFound", err)
	}
}

// TestScopeMemberUpdatesHouseholdRow verifies a non-owner member of a
// household can Update a household row, and the change persists.
func TestScopeMemberUpdatesHouseholdRow(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "owner", "member"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "owner", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(owner): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "member", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(member): %v", err)
	}

	householdAsset := testAsset(func(a *entity.Asset) {
		a.Model = strPtr("ORIG-MODEL")
	})
	householdAsset.OwnerHouseholdID = &h.ID
	created, err := assets.Create(ctx, householdAsset, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household asset: %v", err)
	}

	// Member updates the model on the household row.
	updatedEnt := entity.Asset{ID: created.ID, Model: strPtr("NEW-MODEL")}
	updated, err := assets.Update(ctx, updatedEnt, repo.Owner("member"))
	if err != nil {
		t.Fatalf("member Update household row: %v (want success)", err)
	}
	if updated.Model == nil || *updated.Model != "NEW-MODEL" {
		t.Fatalf("updated Model = %v, want NEW-MODEL", updated.Model)
	}

	// Owner confirms the change persisted.
	got, err := assets.Get(ctx, created.ID, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("owner Get after member Update: %v", err)
	}
	if got.Model == nil || *got.Model != "NEW-MODEL" {
		t.Errorf("owner Get Model = %v, want NEW-MODEL after member update", got.Model)
	}
}

// TestScopeNonOwnerDeleteBlocked verifies only the creator (owner) may Delete:
// a non-owner member's Delete is a no-op (the row remains), and the owner's
// Delete succeeds (subsequent Get is ErrNotFound).
func TestScopeNonOwnerDeleteBlocked(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "owner", "member"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "owner", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(owner): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "member", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(member): %v", err)
	}

	householdAsset := testAsset()
	householdAsset.OwnerHouseholdID = &h.ID
	created, err := assets.Create(ctx, householdAsset, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household asset: %v", err)
	}

	// A member (non-owner) Delete must NOT delete the row.
	if err := assets.Delete(ctx, created.ID, repo.Owner("member")); err != nil {
		t.Fatalf("member Delete household row: %v (want no-op success)", err)
	}
	// The row must still be visible to the owner.
	if _, err := assets.Get(ctx, created.ID, repo.Owner("owner")); err != nil {
		t.Fatalf("owner Get after member Delete: %v (want the row still present)", err)
	}

	// The owner Delete works.
	if err := assets.Delete(ctx, created.ID, repo.Owner("owner")); err != nil {
		t.Fatalf("owner Delete household row: %v", err)
	}
	if _, err := assets.Get(ctx, created.ID, repo.Owner("owner")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("owner Get after owner Delete: err = %v, want ErrNotFound", err)
	}
}

// TestScopeCreateHouseholdByNonMember verifies that a user who is NOT a member
// of a household cannot create a household-scoped row (ErrNotMember), while a
// member can.
func TestScopeCreateHouseholdByNonMember(t *testing.T) {
	assets, households := scopeRepos(t)
	truncateScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePoolFor(t), "owner", "member", "stranger"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	h, err := households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "owner", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(owner): %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "member", repo.Owner("owner")); err != nil {
		t.Fatalf("AddMember(member): %v", err)
	}

	// A non-member attempting to create a household row is rejected.
	strangerAsset := testAsset(func(a *entity.Asset) {
		a.SerialNumber = strPtr("SCOPE-STRANGER")
	})
	strangerAsset.OwnerHouseholdID = &h.ID
	if _, err := assets.Create(ctx, strangerAsset, repo.Owner("stranger")); !errors.Is(err, repo.ErrNotMember) {
		t.Errorf("stranger Create household asset: err = %v, want ErrNotMember", err)
	}

	// A member creating a household asset succeeds.
	memberAsset := testAsset(func(a *entity.Asset) {
		a.SerialNumber = strPtr("SCOPE-MEMBER")
	})
	memberAsset.OwnerHouseholdID = &h.ID
	if _, err := assets.Create(ctx, memberAsset, repo.Owner("member")); err != nil {
		t.Errorf("member Create household asset: err = %v, want success", err)
	}
}
