package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

// finScopeAccounts and finScopeMovements are finance repositories bound to the
// shared p_scope test schema (see scope_test.go). They are built once, on
// first use, on top of the already-initialized scope pool. The scope pool and
// its repos are initialized exactly once via scopeOnce (in scope_test.go);
// this only adds the finance repos.
var (
	finScopeAccounts  *postgres.AccountRepository
	finScopeMovements *postgres.MovementRepository
)

// finScopeFinanceRepos lazily returns the finance repositories bound to the
// shared p_scope test schema.
func finScopeFinanceRepos(t *testing.T) (*postgres.AccountRepository, *postgres.MovementRepository, *postgres.HouseholdRepository) {
	t.Helper()
	// Ensure the scope pool is initialized.
	scopeAssets, scopeHouseholds := scopeRepos(t)
	_ = scopeAssets // unused; just triggers the lazy init

	if finScopeAccounts == nil {
		finScopeAccounts = postgres.NewAccountRepository(scopePool)
	}
	if finScopeMovements == nil {
		finScopeMovements = postgres.NewMovementRepository(scopePool)
	}
	return finScopeAccounts, finScopeMovements, scopeHouseholds
}

// truncateFinanceScope clears all finance + household test data on the shared
// p_scope pool. Call at the top of each test. The Users table is NOT
// truncated: it is the user registry that households, memberships, and
// finance rows reference.
func truncateFinanceScope(t *testing.T) {
	t.Helper()
	finScopeFinanceRepos(t) // ensure the pool is initialized
	if _, err := scopePool.Exec(context.Background(),
		`TRUNCATE household_members, households, financial_accounts, money_movements CASCADE`); err != nil {
		t.Fatalf("truncate finance scope: %v", err)
	}
}

// TestFinanceMemberReadsHouseholdAccount is the core verify for finance
// scope-awareness: a household member (NOT the account's creator) can read a
// household account and its movements and balance, while a non-member cannot.
func TestFinanceMemberReadsHouseholdAccount(t *testing.T) {
	accounts, movements, households := finScopeFinanceRepos(t)
	truncateFinanceScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePool, "alice", "bob", "carol"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	// Alice creates household h and is a member of it (so she may attach a
	// household row to it via the membership trigger).
	h, err := households.Create(ctx, entity.Household{DisplayName: "H"}, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create household: %v", err)
	}
	if err := households.AddMember(ctx, h.ID, "alice", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(alice): %v", err)
	}
	// Bob is added as a member of h. Carol is intentionally NOT a member.
	if err := households.AddMember(ctx, h.ID, "bob", repo.Owner("alice")); err != nil {
		t.Fatalf("AddMember(bob): %v", err)
	}

	// Alice creates a household account A (OwnerHouseholdID = h).
	a := entity.FinancialAccount{
		Name:             "H1 checking",
		Type:             entity.AccountTypeBank,
		Currency:         "USD",
		OwnerHouseholdID: &h.ID,
	}
	created, err := accounts.Create(ctx, a, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create household account: %v", err)
	}
	aID := created.ID

	// Alice creates one movement M with destination = A (kind income, +$50),
	// household-shared.
	m := entity.MoneyMovement{
		Kind:                 entity.KindIncome,
		Amount:               "50.00",
		Currency:             "USD",
		OccurredOn:           time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		Description:          "Salary",
		Origin:               entity.OriginManual,
		DestinationAccountID: &aID,
		OwnerHouseholdID:     &h.ID,
	}
	mCreated, err := movements.Create(ctx, m, repo.Owner("alice"))
	if err != nil {
		t.Fatalf("Create movement: %v", err)
	}
	mID := mCreated.ID

	// ---- bob (member, NOT creator) ----
	if _, err := accounts.Get(ctx, aID, repo.Owner("bob")); err != nil {
		t.Errorf("bob Get household account: err = %v, want success (not ErrNotFound)", err)
	}
	moves, err := movements.MovementsForAccount(ctx, aID, repo.Owner("bob"))
	if err != nil {
		t.Fatalf("bob MovementsForAccount: %v", err)
	}
	found := false
	for _, mv := range moves {
		if mv.ID == mID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("bob MovementsForAccount: missing movement %s (got %d movements)", mID, len(moves))
	}
	bal, err := movements.BalanceForAccount(ctx, aID, repo.Owner("bob"))
	if err != nil {
		t.Fatalf("bob BalanceForAccount: %v", err)
	}
	if bal == "0" {
		t.Errorf("bob BalanceForAccount = %q, want non-zero (movement net)", bal)
	}

	// ---- carol (non-member) ----
	if _, err := accounts.Get(ctx, aID, repo.Owner("carol")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("carol Get household account: err = %v, want ErrNotFound", err)
	}
	cmoves, err := movements.MovementsForAccount(ctx, aID, repo.Owner("carol"))
	if err != nil {
		t.Fatalf("carol MovementsForAccount: %v", err)
	}
	if len(cmoves) != 0 {
		t.Errorf("carol MovementsForAccount = %d rows, want 0 (non-member)", len(cmoves))
	}
	cbal, err := movements.BalanceForAccount(ctx, aID, repo.Owner("carol"))
	if err != nil {
		t.Fatalf("carol BalanceForAccount: %v", err)
	}
	if cbal != "0" {
		t.Errorf("carol BalanceForAccount = %q, want %q (non-member)", cbal, "0")
	}
}

// TestFinancePersonalAccountUnchanged guards against over-sharing: a purely
// personal account (no OwnerHouseholdID) is readable by its owner and NOT by
// another user who shares no household with the owner.
func TestFinancePersonalAccountUnchanged(t *testing.T) {
	accounts, movements, _ := finScopeFinanceRepos(t)
	truncateFinanceScope(t)
	ctx := context.Background()

	if err := ensureUsers(ctx, scopePool, "owner", "stranger"); err != nil {
		t.Fatalf("ensureUsers: %v", err)
	}

	// Owner creates a purely personal account (no OwnerHouseholdID).
	a := entity.FinancialAccount{
		Name:     "Personal",
		Type:     entity.AccountTypeCash,
		Currency: "EUR",
	}
	created, err := accounts.Create(ctx, a, repo.Owner("owner"))
	if err != nil {
		t.Fatalf("Create personal account: %v", err)
	}
	aID := created.ID

	// One movement touching A so MovementsForAccount / BalanceForAccount have
	// something to return.
	m := entity.MoneyMovement{
		Kind:                 entity.KindIncome,
		Amount:               "10.00",
		Currency:             "EUR",
		OccurredOn:           time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Description:          "Piggy bank",
		Origin:               entity.OriginManual,
		DestinationAccountID: &aID,
	}
	if _, err := movements.Create(ctx, m, repo.Owner("owner")); err != nil {
		t.Fatalf("Create movement: %v", err)
	}

	// Owner (creator) can read it.
	if _, err := accounts.Get(ctx, aID, repo.Owner("owner")); err != nil {
		t.Errorf("owner Get personal account: err = %v, want success", err)
	}
	if moves, err := movements.MovementsForAccount(ctx, aID, repo.Owner("owner")); err != nil {
		t.Fatalf("owner MovementsForAccount: %v", err)
	} else if len(moves) != 1 {
		t.Errorf("owner MovementsForAccount = %d rows, want 1", len(moves))
	}
	if bal, err := movements.BalanceForAccount(ctx, aID, repo.Owner("owner")); err != nil {
		t.Fatalf("owner BalanceForAccount: %v", err)
	} else if bal == "0" {
		t.Errorf("owner BalanceForAccount = %q, want non-zero", bal)
	}

	// Stranger (no shared household) cannot.
	if _, err := accounts.Get(ctx, aID, repo.Owner("stranger")); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("stranger Get personal account: err = %v, want ErrNotFound", err)
	}
	if moves, err := movements.MovementsForAccount(ctx, aID, repo.Owner("stranger")); err != nil {
		t.Fatalf("stranger MovementsForAccount: %v", err)
	} else if len(moves) != 0 {
		t.Errorf("stranger MovementsForAccount = %d rows, want 0", len(moves))
	}
	if bal, err := movements.BalanceForAccount(ctx, aID, repo.Owner("stranger")); err != nil {
		t.Fatalf("stranger BalanceForAccount: %v", err)
	} else if bal != "0" {
		t.Errorf("stranger BalanceForAccount = %q, want %q", bal, "0")
	}
}
