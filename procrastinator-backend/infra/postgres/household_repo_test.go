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

const testSchemaHouseholdVisible = "p_household_visible"

var (
	hhVisOnce       sync.Once
	hhVisPool       *pgxpool.Pool
	hhVisHouseholds *postgres.HouseholdRepository
	hhVisInitErr    error
)

// hhVisRepos returns the household repository bound to the p_household_visible
// test schema. Uses the sync.Once lazy init pattern (see scope_test.go): the
// pool and repository are opened and initialized exactly once per test binary
// run.
func hhVisRepos(t *testing.T) *postgres.HouseholdRepository {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	hhVisOnce.Do(func() {
		hhVisPool, hhVisInitErr = openTestPool(t, testSchemaHouseholdVisible)
		if hhVisInitErr != nil {
			return
		}
		hhVisHouseholds = postgres.NewHouseholdRepository(hhVisPool)
	})
	if hhVisInitErr != nil {
		t.Fatalf("init household-visible test pool: %v", hhVisInitErr)
	}
	return hhVisHouseholds
}

// truncateHHVis clears all test data on the household-visible pool. Call at
// the top of each write test. The Users table is NOT truncated: it is the
// user registry that households and memberships reference.
func truncateHHVis(t *testing.T) {
	t.Helper()
	hhVisRepos(t) // ensure the pool is initialized
	if _, err := hhVisPool.Exec(context.Background(),
		`TRUNCATE household_members, households CASCADE`); err != nil {
		t.Fatalf("truncate household-visible: %v", err)
	}
}

// TestHouseholdVisibleForMember verifies the member-aware visibility rule for
// households end to end: a user who is a MEMBER (not the owner) of a household
// can Get and List it, while a stranger (neither owner nor member) sees
// nothing. This is the behavior the generic owner-only Get/List cannot provide
// (households have no owner_household_id column).
func TestHouseholdVisibleForMember(t *testing.T) {
	t.Parallel()
	households := hhVisRepos(t)
	truncateHHVis(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, hhVisPool, userA, userB, "stranger"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	// userA owns the household; userB is added as a member (not owner).
	hh, err := households.Create(ctx, entity.Household{DisplayName: "Shared"}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, hh.ID, userB, repo.Owner(userA)); err != nil {
		t.Fatalf("AddMember(userB): %v", err)
	}

	// As userB (a member, NOT the owner): Get returns the household.
	got, err := households.GetHouseholdVisible(ctx, hh.ID, repo.Owner(userB))
	if err != nil {
		t.Fatalf("member GetHouseholdVisible: %v (want success)", err)
	}
	if got.ID != hh.ID {
		t.Errorf("member GetHouseholdVisible ID = %q, want %q", got.ID, hh.ID)
	}
	if got.OwnerID != userA {
		t.Errorf("member GetHouseholdVisible OwnerID = %q, want %q (the owner)", got.OwnerID, userA)
	}

	// As userB: List includes the household.
	list, err := households.ListHouseholdsVisible(ctx, repo.Owner(userB))
	if err != nil {
		t.Fatalf("member ListHouseholdsVisible: %v", err)
	}
	found := false
	for _, h := range list {
		if h.ID == hh.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("member ListHouseholdsVisible = %v, want it to include %s", list, hh.ID)
	}

	// As a stranger (neither owner nor member): Get is ErrNotFound, List omits it.
	if _, err := households.GetHouseholdVisible(ctx, hh.ID, repo.Owner("stranger")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("stranger GetHouseholdVisible: err = %v, want ErrNotFound", err)
	}
	strangerList, err := households.ListHouseholdsVisible(ctx, repo.Owner("stranger"))
	if err != nil {
		t.Fatalf("stranger ListHouseholdsVisible: %v", err)
	}
	for _, h := range strangerList {
		if h.ID == hh.ID {
			t.Errorf("stranger ListHouseholdsVisible = %v, must NOT include %s", strangerList, hh.ID)
		}
	}
}
