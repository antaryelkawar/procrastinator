package household

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// Test user ids used across the suite.
const (
	uAlice    = "alice"
	uBob      = "bob"
	uCarol    = "carol"
	uStranger = "stranger"
)

var testNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ctxAs(id string) context.Context {
	return user.WithUser(context.Background(), id)
}

// ---------------------------------------------------------------------------
// fake repositories + environment
// ---------------------------------------------------------------------------

// Compile-time interface guards.
var (
	_ repo.HouseholdRepository = (*fakeHouseholdRepo)(nil)
	_ repo.UserRegistry        = (*fakeUserRegistry)(nil)
)

// callCounter counts every public repository method invocation across all fakes.
type callCounter struct {
	mu sync.Mutex
	n  int
}

func (c *callCounter) bump() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *callCounter) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// fakeUserRegistry satisfies repo.UserRegistry. Has reports membership in a
// registered-ID set.
type fakeUserRegistry struct {
	mu    sync.Mutex
	ids   map[string]bool
	calls *callCounter
}

func newFakeUserRegistry(ids ...string) *fakeUserRegistry {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return &fakeUserRegistry{ids: m, calls: &callCounter{}}
}

func (r *fakeUserRegistry) register(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids[id] = true
}

func (r *fakeUserRegistry) Has(ctx context.Context, id string) (bool, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ids[id], nil
}

// fakeHouseholdRepo is an in-memory implementation of
// repo.HouseholdRepository. households maps id -> entity; members maps
// householdID -> ordered unique userIDs.
type fakeHouseholdRepo struct {
	mu         sync.Mutex
	nextID     int
	households map[string]entity.Household
	members    map[string][]string
	calls      *callCounter
}

func newFakeHouseholdRepo(calls *callCounter) *fakeHouseholdRepo {
	return &fakeHouseholdRepo{
		households: make(map[string]entity.Household),
		members:    make(map[string][]string),
		calls:      calls,
	}
}

// seed directly inserts a household + optional members (test setup only, not a
// repository call).
func (r *fakeHouseholdRepo) seed(id, ownerID, name string, members ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.households[id]; !ok {
		r.nextID++
	}
	r.households[id] = entity.Household{ID: id, OwnerID: ownerID, DisplayName: name, CreatedAt: testNow}
	r.members[id] = append([]string{}, members...)
}

func (r *fakeHouseholdRepo) Create(ctx context.Context, h entity.Household, opts ...repo.Option) (entity.Household, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	r.nextID++
	if h.ID == "" {
		h.ID = "hh-" + itoa(r.nextID)
	}
	if h.OwnerID == "" {
		h.OwnerID = o.OwnerID
	}
	if h.CreatedAt.IsZero() {
		h.CreatedAt = testNow
	}
	r.households[h.ID] = h
	return h, nil
}

func (r *fakeHouseholdRepo) AddMember(ctx context.Context, householdID, userID string, opts ...repo.Option) error {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.members[householdID]
	for _, u := range cur {
		if u == userID {
			return nil // idempotent
		}
	}
	r.members[householdID] = append(cur, userID)
	return nil
}

func (r *fakeHouseholdRepo) ListMembers(ctx context.Context, householdID string, opts ...repo.Option) ([]entity.HouseholdMember, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.members[householdID]
	out := make([]entity.HouseholdMember, 0, len(ids))
	for _, uid := range ids {
		out = append(out, entity.HouseholdMember{HouseholdID: householdID, UserID: uid, CreatedAt: testNow})
	}
	return out, nil
}

func (r *fakeHouseholdRepo) HouseholdsForUser(ctx context.Context, userID string, opts ...repo.Option) ([]string, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0)
	for hid, ids := range r.members {
		for _, uid := range ids {
			if uid == userID {
				out = append(out, hid)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *fakeHouseholdRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Household, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.households[id]
	if !ok {
		return entity.Household{}, repo.ErrNotFound
	}
	// RLS visibility for households: owner OR member can see the row.
	if tid := repo.ApplyOptions(opts...).OwnerID; tid != "" {
		if h.OwnerID != tid && !r.isMemberLocked(id, tid) {
			return entity.Household{}, repo.ErrNotFound
		}
	}
	return h, nil
}

// isMemberLocked reports whether uid is in the member list for hid.
// Caller must hold r.mu.
func (r *fakeHouseholdRepo) isMemberLocked(hid, uid string) bool {
	for _, u := range r.members[hid] {
		if u == uid {
			return true
		}
	}
	return false
}

// GetHouseholdVisible has identical member-aware semantics to Get: the owner
// OR a member may see the row; repo.ErrNotFound otherwise.
func (r *fakeHouseholdRepo) GetHouseholdVisible(ctx context.Context, id string, opts ...repo.Option) (entity.Household, error) {
	return r.Get(ctx, id, opts...)
}

// ListHouseholdsVisible returns all households where the requester is the owner
// OR a member, sorted by ID for determinism. Never nil (empty when none).
func (r *fakeHouseholdRepo) ListHouseholdsVisible(ctx context.Context, opts ...repo.Option) ([]entity.Household, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	tid := repo.ApplyOptions(opts...).OwnerID
	out := make([]entity.Household, 0)
	for _, h := range r.households {
		if tid == "" {
			continue
		}
		if h.OwnerID == tid || r.isMemberLocked(h.ID, tid) {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeHouseholdRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Household, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.Household, 0)
	for _, h := range r.households {
		if o.OwnerID != "" && h.OwnerID != o.OwnerID {
			continue
		}
		if !matchHouseholdFilter(h, o.Filters) {
			continue
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeHouseholdRepo) Update(ctx context.Context, h entity.Household, opts ...repo.Option) (entity.Household, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.households[h.ID] = h
	return h, nil
}

func (r *fakeHouseholdRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.households[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.households, id)
	delete(r.members, id)
	return nil
}

// Exists reports whether the household exists at all, independent of the
// caller's membership (mirrors the RLS-bypassing repository probe).
func (r *fakeHouseholdRepo) Exists(ctx context.Context, id string, opts ...repo.Option) (bool, error) {
	r.calls.bump()
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.households[id]
	return ok, nil
}

// memberCount is a test assertion helper.
func (r *fakeHouseholdRepo) memberCount(hid string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members[hid])
}

func (r *fakeHouseholdRepo) memberIDs(hid string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.members[hid]...)
}

func matchHouseholdFilter(h entity.Household, filters []repo.Filter) bool {
	for _, f := range filters {
		if f.Op != "=" {
			return false
		}
		want, ok := f.Value.(string)
		if !ok {
			return false
		}
		switch f.Field {
		case "id":
			if h.ID != want {
				return false
			}
		case "owner_id":
			if h.OwnerID != want {
				return false
			}
		case "display_name":
			if h.DisplayName != want {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// environment
// ---------------------------------------------------------------------------

type testEnv struct {
	svc        *Service
	households *fakeHouseholdRepo
	users      *fakeUserRegistry
	calls      *callCounter
}

func newTestEnv() *testEnv {
	calls := &callCounter{}
	households := newFakeHouseholdRepo(calls)
	users := newFakeUserRegistry()
	factory := &repo.Factory{
		Households: households,
		Users:      users,
		InTx: func(ctx context.Context, fn func(context.Context, *repo.Repos) error) error {
			return fn(ctx, &repo.Repos{Households: households})
		},
	}
	return &testEnv{
		svc:        New(factory),
		households: households,
		users:      users,
		calls:      calls,
	}
}

// registerUsers seeds the registry (not a repository call).
func (e *testEnv) registerUsers(ids ...string) {
	for _, id := range ids {
		e.users.register(id)
	}
}

// ---------------------------------------------------------------------------
// CreateHousehold
// ---------------------------------------------------------------------------

func TestCreateHousehold_OwnedByUserAndCreatorMember(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice)
	ctx := ctxAs(uAlice)

	h, err := e.svc.CreateHousehold(ctx, "The Smiths")
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if h.ID == "" {
		t.Fatal("expected a non-empty generated ID")
	}
	if h.OwnerID != uAlice {
		t.Fatalf("OwnerID = %q, want %q", h.OwnerID, uAlice)
	}
	if h.DisplayName != "The Smiths" {
		t.Fatalf("DisplayName = %q, want %q", h.DisplayName, "The Smiths")
	}

	members, err := e.households.ListMembers(ctx, h.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != uAlice {
		t.Fatalf("members = %+v, want creator %q auto-added", members, uAlice)
	}
}

func TestCreateHousehold_BlankName(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice)
	for _, name := range []string{"", "   ", "\t\n"} {
		before := e.calls.total()
		_, err := e.svc.CreateHousehold(ctxAs(uAlice), name)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("name %q: err = %v, want ErrInvalid", name, err)
		}
		if e.calls.total() != before {
			t.Fatalf("name %q: repository calls changed (%d -> %d), want none", name, before, e.calls.total())
		}
	}
}

// ---------------------------------------------------------------------------
// AddMember
// ---------------------------------------------------------------------------

func TestAddMember_UnknownHousehold(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice, uBob)
	e.households.seed("h1", uAlice, "H1", uAlice)
	ctx := ctxAs(uAlice)

	err := e.svc.AddMember(ctx, "nope", uBob)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound (unknown household)", err)
	}
	if got := e.households.memberIDs("h1"); len(got) != 1 {
		t.Fatalf("memberIDs = %v, want unchanged [alice]", got)
	}
}

func TestAddMember_ByMember(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice, uBob)
	// alice owns h1 and is a member; add bob (a registered user).
	e.households.seed("h1", uAlice, "H1", uAlice)
	ctx := ctxAs(uAlice)

	if err := e.svc.AddMember(ctx, "h1", uBob); err != nil {
		t.Fatalf("AddMember(bob): %v", err)
	}
	memberIDs := e.households.memberIDs("h1")
	if len(memberIDs) != 2 || memberIDs[0] != uAlice || memberIDs[1] != uBob {
		t.Fatalf("memberIDs = %v, want [alice bob]", memberIDs)
	}

	// Idempotent: adding bob again is a no-op.
	if err := e.svc.AddMember(ctx, "h1", uBob); err != nil {
		t.Fatalf("AddMember(bob) again: %v", err)
	}
	if got := e.households.memberIDs("h1"); len(got) != 2 {
		t.Fatalf("memberIDs after re-add = %v, want no duplicate (len 2)", got)
	}
}

func TestAddMember_ByNonMember(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice, uBob)
	e.households.seed("h1", uAlice, "H1", uAlice)
	ctx := ctxAs(uBob) // bob is not a member of h1

	err := e.svc.AddMember(ctx, "h1", uCarol)
	if !errors.Is(err, ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
	if got := e.households.memberIDs("h1"); len(got) != 1 {
		t.Fatalf("memberIDs = %v, want unchanged [alice]", got)
	}
}

func TestAddMember_UnregisteredUser(t *testing.T) {
	e := newTestEnv()
	e.registerUsers(uAlice) // carol NOT registered
	e.households.seed("h1", uAlice, "H1", uAlice)
	ctx := ctxAs(uAlice)

	err := e.svc.AddMember(ctx, "h1", uCarol)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound", err)
	}
	if got := e.households.memberIDs("h1"); len(got) != 1 {
		t.Fatalf("memberIDs = %v, want unchanged [alice]", got)
	}
}

// ---------------------------------------------------------------------------
// IsMember
// ---------------------------------------------------------------------------

func TestIsMember(t *testing.T) {
	e := newTestEnv()
	e.households.seed("h1", uAlice, "H1", uAlice, uBob)

	if ok, err := e.svc.IsMember(ctxAs(uAlice), "h1"); err != nil || !ok {
		t.Fatalf("IsMember(alice) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := e.svc.IsMember(ctxAs(uBob), "h1"); err != nil || !ok {
		t.Fatalf("IsMember(bob) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := e.svc.IsMember(ctxAs(uCarol), "h1"); err != nil || ok {
		t.Fatalf("IsMember(carol) = %v, %v; want false, nil", ok, err)
	}
}

// ---------------------------------------------------------------------------
// GetHousehold
// ---------------------------------------------------------------------------

func TestGetHousehold_ByMemberWithMembers(t *testing.T) {
	e := newTestEnv()
	e.households.seed("h1", uAlice, "H1", uAlice, uBob)

	got, err := e.svc.GetHousehold(ctxAs(uBob), "h1") // member
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if got.Household.ID != "h1" || got.Household.OwnerID != uAlice {
		t.Fatalf("Household = %+v, want id h1 owner alice", got.Household)
	}
	if len(got.Members) != 2 {
		t.Fatalf("Members = %+v, want 2", got.Members)
	}
}

func TestGetHousehold_ByOwner(t *testing.T) {
	e := newTestEnv()
	// Owner is also auto a member in practice, but test owner path explicitly.
	e.households.seed("h1", uAlice, "H1", uAlice)

	got, err := e.svc.GetHousehold(ctxAs(uAlice), "h1")
	if err != nil {
		t.Fatalf("GetHousehold(owner): %v", err)
	}
	if got.Household.OwnerID != uAlice {
		t.Fatalf("OwnerID = %q, want alice", got.Household.OwnerID)
	}
}

func TestGetHousehold_ByNonMember(t *testing.T) {
	e := newTestEnv()
	e.households.seed("h1", uAlice, "H1", uAlice, uBob)

	_, err := e.svc.GetHousehold(ctxAs(uCarol), "h1")
	if !errors.Is(err, ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestGetHousehold_UnknownID(t *testing.T) {
	e := newTestEnv()
	e.households.seed("h1", uAlice, "H1", uAlice)

	_, err := e.svc.GetHousehold(ctxAs(uAlice), "nope")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// ListMyHouseholds
// ---------------------------------------------------------------------------

func TestListMyHouseholds_OwnedPlusMembered(t *testing.T) {
	e := newTestEnv()
	// h-owned: alice owns it, alice is a member.
	e.households.seed("h-owned", uAlice, "Owned", uAlice)
	// h-member: bob owns, alice is a member (not owner).
	e.households.seed("h-member", uBob, "Membered", uBob, uAlice)
	// h-stranger: alice is neither owner nor member.
	e.households.seed("h-stranger", uCarol, "Stranger", uCarol)

	got, err := e.svc.ListMyHouseholds(ctxAs(uAlice))
	if err != nil {
		t.Fatalf("ListMyHouseholds: %v", err)
	}

	byID := make(map[string]HouseholdWithMembers, len(got))
	for _, item := range got {
		byID[item.Household.ID] = item
	}
	if _, ok := byID["h-owned"]; !ok {
		t.Fatalf("missing owned household; got ids %v", idList(got))
	}
	if _, ok := byID["h-member"]; !ok {
		t.Fatalf("missing membered household; got ids %v", idList(got))
	}
	if _, ok := byID["h-stranger"]; ok {
		t.Fatalf("must not include stranger household; got ids %v", idList(got))
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (deduped owned+membered); ids %v", len(got), idList(got))
	}

	// Each returned item carries its members.
	if got := byID["h-owned"]; len(got.Members) != 1 || got.Members[0].UserID != uAlice {
		t.Fatalf("h-owned members = %+v, want [alice]", got.Members)
	}
	if got := byID["h-member"]; len(got.Members) != 2 {
		t.Fatalf("h-member members = %+v, want 2", got.Members)
	}
}

func TestListMyHouseholds_Empty(t *testing.T) {
	e := newTestEnv()
	got, err := e.svc.ListMyHouseholds(ctxAs(uAlice))
	if err != nil {
		t.Fatalf("ListMyHouseholds: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// Fail-closed (no user in context)
// ---------------------------------------------------------------------------

func TestFailClosed_NoUser(t *testing.T) {
	run := func(t *testing.T, e *testEnv, fn func(context.Context) error) {
		t.Helper()
		e.registerUsers(uAlice, uBob)
		e.households.seed("h1", uAlice, "H1", uAlice)
		before := e.calls.total()
		err := fn(context.Background()) // no user bound
		if !errors.Is(err, user.ErrNoUser) {
			t.Fatalf("err = %v, want user.ErrNoUser", err)
		}
		if e.calls.total() != before {
			t.Fatalf("repository calls changed (%d -> %d), want zero", before, e.calls.total())
		}
	}

	t.Run("CreateHousehold", func(t *testing.T) {
		e := newTestEnv()
		run(t, e, func(ctx context.Context) error {
			_, err := e.svc.CreateHousehold(ctx, "X")
			return err
		})
	})
	t.Run("AddMember", func(t *testing.T) {
		e := newTestEnv()
		run(t, e, func(ctx context.Context) error {
			return e.svc.AddMember(ctx, "h1", uBob)
		})
	})
	t.Run("IsMember", func(t *testing.T) {
		e := newTestEnv()
		run(t, e, func(ctx context.Context) error {
			_, err := e.svc.IsMember(ctx, "h1")
			return err
		})
	})
	t.Run("GetHousehold", func(t *testing.T) {
		e := newTestEnv()
		run(t, e, func(ctx context.Context) error {
			_, err := e.svc.GetHousehold(ctx, "h1")
			return err
		})
	})
	t.Run("ListMyHouseholds", func(t *testing.T) {
		e := newTestEnv()
		run(t, e, func(ctx context.Context) error {
			_, err := e.svc.ListMyHouseholds(ctx)
			return err
		})
	})
}

func idList(items []HouseholdWithMembers) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Household.ID)
	}
	return out
}
