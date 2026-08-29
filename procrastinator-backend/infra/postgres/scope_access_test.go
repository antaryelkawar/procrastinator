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
	"procrastinator-backend/commons/tenant"
)

// listScopedQuerier extends recordingQuerier with a canned household-membership
// result: the household query is the FIRST query issued, so the recorded
// sql/args are from the membership call; the List SQL is captured separately
// as listSQL/listArgs.
type listScopedQuerier struct {
	households []string
	hhErr      error

	listSQL  string
	listArgs []any
	queried  bool
	queryErr error
	rowErr   error // returned by Rows.Err(); nil = no error
}

func (q *listScopedQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (q *listScopedQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !q.queried {
		// First query is the household-membership lookup.
		q.queried = true
		return &scopedHHRows{q: q}, nil
	}
	q.listSQL = sql
	q.listArgs = args
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	return scopedListRows{q: q}, nil
}

func (q *listScopedQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return errorRow{err: pgx.ErrNoRows}
}

// scopedHHRows is a pgx.Rows over the canned household ids.
type scopedHHRows struct {
	q   *listScopedQuerier
	idx int // next household to hand out
	cur int // household handed out by the most recent Next
}

func (r scopedHHRows) Close()                                       {}
func (r scopedHHRows) Err() error                                   { return r.q.hhErr }
func (r scopedHHRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *scopedHHRows) Next() bool {
	if r.idx >= len(r.q.households) {
		return false
	}
	r.cur = r.idx
	r.idx++
	return true
}
func (r *scopedHHRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("scopedHHRows: want 1 dest, got %d", len(dest))
	}
	p, ok := dest[0].(*string)
	if !ok {
		return fmt.Errorf("scopedHHRows: dest[0] = %T, want *string", dest[0])
	}
	*p = r.q.households[r.cur]
	return nil
}
func (r scopedHHRows) Values() ([]any, error) { return nil, nil }
func (r scopedHHRows) RawValues() [][]byte    { return nil }
func (r scopedHHRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}
func (r scopedHHRows) Conn() *pgx.Conn { return nil }

// scopedListRows is a pgx.Rows with no data rows (List returns empty).
type scopedListRows struct {
	q *listScopedQuerier
}

func (r scopedListRows) Close()                                       {}
func (r scopedListRows) Err() error                                   { return r.q.rowErr }
func (r scopedListRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r scopedListRows) Next() bool                                   { return false }
func (r scopedListRows) Scan(dest ...any) error                       { return pgx.ErrNoRows }
func (r scopedListRows) Values() ([]any, error)                       { return nil, nil }
func (r scopedListRows) RawValues() [][]byte                          { return nil }
func (r scopedListRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r scopedListRows) Conn() *pgx.Conn                              { return nil }

func scopeAssetRepo(q Querier) *pgRepository[entity.Asset] {
	return &pgRepository[entity.Asset]{
		scope:   &noopScope{q: q},
		table:   "assets",
		scanRow: scanAsset,
		toMap:   assetToMap,
		filters: assetFilters,
	}
}

// TestListScoped_NoHouseholds verifies the spec rule for a user with no
// household memberships: the query is tenant-scoped AND restricted to
// personal rows, with no empty IN () anywhere.
func TestListScoped_NoHouseholds(t *testing.T) {
	t.Parallel()

	q := &listScopedQuerier{}
	r := scopeAssetRepo(q)

	res, err := r.ListScoped(context.Background(), repo.Tenant("acme"))
	if err != nil {
		t.Fatalf("ListScoped: %v", err)
	}
	if res == nil {
		t.Fatal("ListScoped = nil, want non-nil empty slice")
	}
	if len(res) != 0 {
		t.Fatalf("len = %d, want 0", len(res))
	}

	if !q.queried {
		t.Fatal("household-membership query was not issued")
	}
	if !strings.Contains(q.listSQL, "tenant_id = $1") {
		t.Fatalf("list sql = %q, want tenant_id = $1", q.listSQL)
	}
	if !strings.Contains(q.listSQL, "scope_type = 'personal'") {
		t.Fatalf("list sql = %q, want scope_type = 'personal' condition", q.listSQL)
	}
	if strings.Contains(q.listSQL, "IN ()") {
		t.Fatalf("list sql = %q, must not contain empty IN ()", q.listSQL)
	}
	if !strings.Contains(q.listSQL, "SELECT * FROM assets") {
		t.Fatalf("list sql = %q, want SELECT * FROM assets", q.listSQL)
	}
	if len(q.listArgs) != 1 || q.listArgs[0] != "acme" {
		t.Fatalf("list args = %v, want [acme]", q.listArgs)
	}
}

// TestListScoped_WithHouseholds verifies the spec rule for a user in
// households: personal rows plus rows owned by any of the user's households,
// with one bind placeholder per household id.
func TestListScoped_WithHouseholds(t *testing.T) {
	t.Parallel()

	q := &listScopedQuerier{households: []string{"hh-1", "hh-2"}}
	r := scopeAssetRepo(q)

	_, err := r.ListScoped(context.Background(), repo.Tenant("acme"))
	if err != nil {
		t.Fatalf("ListScoped: %v", err)
	}

	if !strings.Contains(q.listSQL, "tenant_id = $1") {
		t.Fatalf("list sql = %q, want tenant_id = $1", q.listSQL)
	}
	if !strings.Contains(q.listSQL, "(scope_type = 'personal' OR owner_household_id IN ($2, $3))") {
		t.Fatalf("list sql = %q, want scope condition with one $-placeholder per household", q.listSQL)
	}
	want := []any{"acme", "hh-1", "hh-2"}
	if len(q.listArgs) != len(want) {
		t.Fatalf("list args = %v, want %v", q.listArgs, want)
	}
	for i := range want {
		if q.listArgs[i] != want[i] {
			t.Fatalf("list args = %v, want %v", q.listArgs, want)
		}
	}
	if strings.Contains(q.listSQL, "hh-1") || strings.Contains(q.listSQL, "hh-2") {
		t.Fatalf("list sql = %q, must not interpolate household ids", q.listSQL)
	}
}

// TestListScoped_Pagination verifies that OrderBy, Limit, and Offset are
// applied after the scope condition exactly like the generic List.
func TestListScoped_Pagination(t *testing.T) {
	t.Parallel()

	q := &listScopedQuerier{households: []string{"hh-1"}}
	r := scopeAssetRepo(q)

	_, err := r.ListScoped(context.Background(),
		repo.Tenant("acme"), repo.OrderBy("created_at, id"), repo.Limit(10), repo.Offset(5))
	if err != nil {
		t.Fatalf("ListScoped: %v", err)
	}

	if !strings.Contains(q.listSQL, "ORDER BY created_at, id") {
		t.Fatalf("list sql = %q, want ORDER BY created_at, id", q.listSQL)
	}
	if !strings.Contains(q.listSQL, "LIMIT $3") {
		t.Fatalf("list sql = %q, want LIMIT $3", q.listSQL)
	}
	if !strings.Contains(q.listSQL, "OFFSET $4") {
		t.Fatalf("list sql = %q, want OFFSET $4", q.listSQL)
	}
	want := []any{"acme", "hh-1", 10, 5}
	if len(q.listArgs) != len(want) {
		t.Fatalf("list args = %v, want %v", q.listArgs, want)
	}
	for i := range want {
		if q.listArgs[i] != want[i] {
			t.Fatalf("list args = %v, want %v", q.listArgs, want)
		}
	}
}

// TestListScoped_FiltersAppended verifies that caller filters are validated
// and appended after the scope condition.
func TestListScoped_FiltersAppended(t *testing.T) {
	t.Parallel()

	q := &listScopedQuerier{households: []string{"hh-1"}}
	r := scopeAssetRepo(q)

	_, err := r.ListScoped(context.Background(),
		repo.Tenant("acme"), repo.Where("doc_type", "=", "invoice"))
	if err != nil {
		t.Fatalf("ListScoped: %v", err)
	}

	if !strings.Contains(q.listSQL, "doc_type = $3") {
		t.Fatalf("list sql = %q, want doc_type = $3 after the scope condition", q.listSQL)
	}
	if !strings.HasSuffix(q.listSQL, "AND doc_type = $3") {
		t.Fatalf("list sql = %q, want caller filter appended last", q.listSQL)
	}
	want := []any{"acme", "hh-1", "invoice"}
	if len(q.listArgs) != len(want) {
		t.Fatalf("list args = %v, want %v", q.listArgs, want)
	}
	for i := range want {
		if q.listArgs[i] != want[i] {
			t.Fatalf("list args = %v, want %v", q.listArgs, want)
		}
	}
}

// TestListScoped_ValidationErrors verifies that unknown filter fields, unknown
// order columns, and a missing tenant are rejected before any query is
// issued (zero SQL).
func TestListScoped_ValidationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    []repo.Option
		wantErr string
	}{
		{"no tenant", nil, tenant.ErrNoTenant.Error()},
		{"unknown filter field", []repo.Option{
			repo.Tenant("acme"), repo.Where("1; DROP TABLE assets--", "=", "x"),
		}, "unknown filter field"},
		{"unknown order column", []repo.Option{
			repo.Tenant("acme"), repo.OrderBy("secret"),
		}, "unknown order column"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &panicQuerier{}
			r := scopeAssetRepo(q)
			_, err := r.ListScoped(context.Background(), tc.opts...)
			if err == nil {
				t.Fatal("err = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestListScoped_RowError verifies that row-level errors from the household
// membership lookup propagate out of ListScoped.
func TestListScoped_RowError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom: row error")
	q := &listScopedQuerier{households: []string{"hh-1"}, rowErr: wantErr}
	r := scopeAssetRepo(q)

	_, err := r.ListScoped(context.Background(), repo.Tenant("acme"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

// TestHouseholdsForUser verifies the helper returns the user's household ids,
// an empty (non-nil) slice when there are none, and propagates row errors.
func TestHouseholdsForUser(t *testing.T) {
	t.Parallel()

	t.Run("with households", func(t *testing.T) {
		t.Parallel()
		q := &listScopedQuerier{households: []string{"hh-1", "hh-2"}}
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
		q := &listScopedQuerier{}
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
		q := &listScopedQuerier{hhErr: wantErr}
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
