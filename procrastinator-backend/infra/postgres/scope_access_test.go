package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// visQuerier is a fake Querier for unit-driving the engine's scope-aware
// methods (List/Get/Update) through the generic pgRepository. The first Query
// is the household-membership lookup (its canned rows are served by
// visHHRows); every other query (the actual SELECT/UPDATE) records its SQL and
// args as lastSQL/lastArgs so tests can assert on the generated visibility
// fragment. The membership lookup is always issued before the data query, so
// the "first query" is unambiguously the membership call.
type visQuerier struct {
	households []string
	hhErr      error
	firstQuery bool // true until the membership lookup has been served

	// lastSQL/lastArgs capture the non-membership query.
	lastSQL  string
	lastArgs []any

	// rowErr is surfaced by the data rows' Err() to test error propagation.
	rowErr error
}

func (q *visQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.lastSQL, q.lastArgs = sql, args
	return pgconn.CommandTag{}, nil
}

func (q *visQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if q.firstQuery {
		q.firstQuery = false
		return &visHHRows{q: q}, nil
	}
	q.lastSQL, q.lastArgs = sql, args
	return visDataRows{q: q}, nil
}

func (q *visQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	// A Get/Update against a fake that has no data returns ErrNoRows; the
	// membership lookup is a Query, so QueryRow here is the data fetch.
	q.lastSQL, q.lastArgs = sql, args
	return errorRow{err: pgx.ErrNoRows}
}

// visHHRows is a pgx.Rows over the canned household ids.
type visHHRows struct {
	q   *visQuerier
	idx int
	cur int
}

func (r visHHRows) Close()                                       {}
func (r visHHRows) Err() error                                   { return r.q.hhErr }
func (r visHHRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *visHHRows) Next() bool {
	if r.idx >= len(r.q.households) {
		return false
	}
	r.cur = r.idx
	r.idx++
	return true
}
func (r *visHHRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("visHHRows: want 1 dest, got %d", len(dest))
	}
	p, ok := dest[0].(*string)
	if !ok {
		return fmt.Errorf("visHHRows: dest[0] = %T, want *string", dest[0])
	}
	*p = r.q.households[r.cur]
	return nil
}
func (r visHHRows) Values() ([]any, error) { return nil, nil }
func (r visHHRows) RawValues() [][]byte    { return nil }
func (r visHHRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}
func (r visHHRows) Conn() *pgx.Conn { return nil }

// visDataRows is a pgx.Rows with no data rows (List returns empty / Get no row).
type visDataRows struct {
	q *visQuerier
}

func (r visDataRows) Close()                                       {}
func (r visDataRows) Err() error                                   { return r.q.rowErr }
func (r visDataRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r visDataRows) Next() bool                                   { return false }
func (r visDataRows) Scan(dest ...any) error                       { return pgx.ErrNoRows }
func (r visDataRows) Values() ([]any, error)                       { return nil, nil }
func (r visDataRows) RawValues() [][]byte                          { return nil }
func (r visDataRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r visDataRows) Conn() *pgx.Conn                              { return nil }

// visAssetRepo builds a shareable asset repository (the generic engine) over the
// given querier, mirroring NewAssetRepository but with a noop scope so no real
// DB is touched.
func visAssetRepo(q Querier) *pgRepository[entity.Asset] {
	return &pgRepository[entity.Asset]{
		scope:     &noopScope{q: q},
		table:     "assets",
		scanRow:   scanAsset,
		codec:     assetCodec,
		shareable: true,
	}
}

// visHouseholdRepo builds a non-shareable household repository (the generic
// engine), mirroring NewHouseholdRepository with a noop scope.
func visHouseholdRepo(q Querier) *pgRepository[entity.Household] {
	return &pgRepository[entity.Household]{
		scope:   &noopScope{q: q},
		table:   "households",
		scanRow: scanHousehold,
		codec:   householdCodec,
		// shareable intentionally left false (zero value).
	}
}

// TestVisibility_ListWithHouseholds verifies the List SQL for a shareable
// repository whose user belongs to two households: the fragment is
// (owner_id = $1 OR owner_household_id IN ($2, $3)) with one bind arg per
// household id and no scope_type remnants.
func TestVisibility_ListWithHouseholds(t *testing.T) {
	t.Parallel()

	q := &visQuerier{households: []string{"hh-1", "hh-2"}, firstQuery: true}
	r := visAssetRepo(q)

	_, err := r.List(context.Background(), repo.Owner("acme"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "(owner_id = $1 OR owner_household_id IN ($2, $3))"
	if !strings.Contains(q.lastSQL, want) {
		t.Fatalf("sql = %q, want fragment %q", q.lastSQL, want)
	}
	if strings.Contains(q.lastSQL, "scope_type") {
		t.Fatalf("sql = %q, must not reference scope_type", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "IN ()") {
		t.Fatalf("sql = %q, must not contain empty IN ()", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "SELECT id, owner_id, owner_household_id, deleted_at, created_at, updated_at, payload FROM assets") {
		t.Fatalf("sql = %q, want explicit column list FROM assets", q.lastSQL)
	}
	got := []any(q.lastArgs)
	wantArgs := []any{"acme", "hh-1", "hh-2"}
	if len(got) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", got, wantArgs)
	}
	for i := range wantArgs {
		if got[i] != wantArgs[i] {
			t.Fatalf("args = %v, want %v", got, wantArgs)
		}
	}
	if strings.Contains(q.lastSQL, "hh-1") || strings.Contains(q.lastSQL, "hh-2") {
		t.Fatalf("sql = %q, must not interpolate household ids", q.lastSQL)
	}
}

// TestVisibility_ListNoHouseholds verifies the List SQL for a shareable
// repository whose user belongs to NO household: only owner_id = $1, with no
// IN clause at all and a single bind arg.
func TestVisibility_ListNoHouseholds(t *testing.T) {
	t.Parallel()

	q := &visQuerier{firstQuery: true}
	r := visAssetRepo(q)

	_, err := r.List(context.Background(), repo.Owner("acme"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.lastSQL, "owner_id = $1") {
		t.Fatalf("sql = %q, want owner_id = $1", q.lastSQL)
	}
	// The WHERE clause (not the SELECT list, which carries the
	// owner_household_id column) must not reference the household disjunct.
	where := q.lastSQL
	if i := strings.Index(q.lastSQL, " WHERE "); i >= 0 {
		where = q.lastSQL[i+len(" WHERE "):]
	}
	if strings.Contains(where, "owner_household_id") {
		t.Fatalf("where = %q, must not reference owner_household_id for a user with no households", where)
	}
	if strings.Contains(where, "IN (") {
		t.Fatalf("where = %q, must not contain an IN clause for a user with no households", where)
	}
	if strings.Contains(q.lastSQL, "scope_type") {
		t.Fatalf("sql = %q, must not reference scope_type", q.lastSQL)
	}
	if len(q.lastArgs) != 1 || q.lastArgs[0] != "acme" {
		t.Fatalf("args = %v, want [acme]", q.lastArgs)
	}
}

// TestVisibility_Get verifies the Get SQL keeps id = $1 AND the visibility
// fragment (owner_id = $2 OR owner_household_id IN ($3)) for a user with one
// household.
func TestVisibility_Get(t *testing.T) {
	t.Parallel()

	q := &visQuerier{households: []string{"hh-1"}, firstQuery: true}
	r := visAssetRepo(q)

	// Get returns ErrNotFound (fake row has no data); we only assert on the
	// generated SQL and args.
	_, err := r.Get(context.Background(), "the-id", repo.Owner("acme"))
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Get: err = %v, want ErrNotFound from fake row", err)
	}

	if !strings.Contains(q.lastSQL, "id = $1") {
		t.Fatalf("sql = %q, want id = $1", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "(owner_id = $2 OR owner_household_id IN ($3))") {
		t.Fatalf("sql = %q, want (owner_id = $2 OR owner_household_id IN ($3))", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "scope_type") {
		t.Fatalf("sql = %q, must not reference scope_type", q.lastSQL)
	}
	wantArgs := []any{"the-id", "acme", "hh-1"}
	if len(q.lastArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", q.lastArgs, wantArgs)
	}
	for i := range wantArgs {
		if q.lastArgs[i] != wantArgs[i] {
			t.Fatalf("args = %v, want %v", q.lastArgs, wantArgs)
		}
	}
}

// TestVisibility_DeleteOwnerOnly verifies Delete is strictly owner-scoped:
// the SQL is id = $1 AND owner_id = $2 with NO household disjunct, and the
// membership lookup is NOT consulted (args = [id, user] only).
func TestVisibility_DeleteOwnerOnly(t *testing.T) {
	t.Parallel()

	q := &visQuerier{households: []string{"hh-1", "hh-2"}}
	r := visAssetRepo(q)

	if err := r.Delete(context.Background(), "the-id", repo.Owner("acme")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if !strings.Contains(q.lastSQL, "id = $1 AND owner_id = $2") {
		t.Fatalf("sql = %q, want id = $1 AND owner_id = $2", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "owner_household_id") {
		t.Fatalf("sql = %q, must NOT include the household disjunct", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "OR") {
		t.Fatalf("sql = %q, Delete must be owner-only (no OR disjunct)", q.lastSQL)
	}
	// Delete does not consult household membership: only id + user args.
	if len(q.lastArgs) != 2 {
		t.Fatalf("args = %v, want exactly [id, user] (no membership args)", q.lastArgs)
	}
	if q.lastArgs[0] != "the-id" || q.lastArgs[1] != "acme" {
		t.Fatalf("args = %v, want [the-id, acme]", q.lastArgs)
	}
}

// TestVisibility_NonShareable verifies a non-shareable repository (household
// shape, shareable=false) emits owner_id = $1 with NO owner_household_id
// disjunct, even for a user who belongs to households.
func TestVisibility_NonShareable(t *testing.T) {
	t.Parallel()

	q := &visQuerier{households: []string{"hh-1", "hh-2"}}
	r := visHouseholdRepo(q)

	_, err := r.List(context.Background(), repo.Owner("acme"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.lastSQL, "owner_id = $1") {
		t.Fatalf("sql = %q, want owner_id = $1", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "owner_household_id") {
		t.Fatalf("sql = %q, non-shareable repo must not reference owner_household_id", q.lastSQL)
	}
	if strings.Contains(q.lastSQL, "IN (") {
		t.Fatalf("sql = %q, non-shareable repo must not emit an IN clause", q.lastSQL)
	}
	if len(q.lastArgs) != 1 || q.lastArgs[0] != "acme" {
		t.Fatalf("args = %v, want [acme] (membership not consulted)", q.lastArgs)
	}
}

// TestHouseholdsForUser verifies the helper returns the user's household ids,
// an empty (non-nil) slice when there are none, and propagates row errors.
func TestHouseholdsForUser(t *testing.T) {
	t.Parallel()

	t.Run("with households", func(t *testing.T) {
		t.Parallel()
		q := &visQuerier{households: []string{"hh-1", "hh-2"}, firstQuery: true}
		got, err := householdsForUser(context.Background(), q, "acme")
		if err != nil {
			t.Fatalf("householdsForUser: %v", err)
		}
		want := []string{"hh-1", "hh-2"}
		if len(got) != len(want) {
			t.Fatalf("got = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got = %v, want %v", got, want)
			}
		}
	})

	t.Run("no households", func(t *testing.T) {
		t.Parallel()
		q := &visQuerier{firstQuery: true}
		got, err := householdsForUser(context.Background(), q, "acme")
		if err != nil {
			t.Fatalf("householdsForUser: %v", err)
		}
		if got == nil {
			t.Fatal("got = nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Fatalf("got = %v, want empty", got)
		}
	})

	t.Run("row error propagates", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("boom")
		q := &visQuerier{hhErr: wantErr, firstQuery: true}
		_, err := householdsForUser(context.Background(), q, "acme")
		if !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
	})
}

// TestAssetSourceWrappers verifies the concrete wrapper types satisfy the
// generic repository interface and that the constructors return them.
func TestAssetSourceWrappers(t *testing.T) {
	t.Parallel()

	var _ repo.Repository[entity.Asset] = (*AssetRepository)(nil)
	var _ repo.Repository[entity.Source] = (*SourceRepository)(nil)

	_ = NewAssetRepository(nil)
	_ = NewSourceRepository(nil)
}
