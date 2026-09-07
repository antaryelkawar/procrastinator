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
	"procrastinator-backend/commons/user"
)

// panicQuerier panics on any query — used to verify zero SQL on ErrNouser.
type panicQuerier struct{}

func (p *panicQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	panic("panicQuerier: Exec called — no SQL should be issued")
}

func (p *panicQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	panic("panicQuerier: Query called — no SQL should be issued")
}

func (p *panicQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	panic("panicQuerier: QueryRow called — no SQL should be issued")
}

// noopScope is a txScope for unit tests that delegates to a fixed Querier
// without any transaction or set_config.
type noopScope struct {
	q Querier
}

func (s *noopScope) run(_ context.Context, _ string, fn func(q Querier) error) error {
	return fn(s.q)
}

// TestNoUser_NoSQL verifies that every repository method fails closed with
// user.ErrNoUser when no user is resolvable, issuing zero SQL.
func TestNoUser_NoSQL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(r *pgRepository[entity.Asset]) (any, error)
	}{
		{"Get", func(r *pgRepository[entity.Asset]) (any, error) {
			return r.Get(context.Background(), "some-id")
		}},
		{"List", func(r *pgRepository[entity.Asset]) (any, error) {
			return r.List(context.Background())
		}},
		{"Create", func(r *pgRepository[entity.Asset]) (any, error) {
			return r.Create(context.Background(), entity.Asset{Brand: &[]string{"x"}[0]})
		}},
		{"Update", func(r *pgRepository[entity.Asset]) (any, error) {
			return r.Update(context.Background(), entity.Asset{ID: "some-id"})
		}},
		{"Delete", func(r *pgRepository[entity.Asset]) (any, error) {
			return struct{}{}, r.Delete(context.Background(), "some-id")
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &pgRepository[entity.Asset]{scope: &noopScope{q: &panicQuerier{}}}
			_, err := tc.call(r)
			if !errors.Is(err, user.ErrNoUser) {
				t.Fatalf("err = %v, want %v", err, user.ErrNoUser)
			}
		})
	}
}

// recordingQuerier records the SQL and args of the first call so tests can
// assert on user scoping without a database.
type recordingQuerier struct {
	sql  string
	args []any
}

func (r *recordingQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	r.sql, r.args = sql, args
	return pgconn.CommandTag{}, nil
}

func (r *recordingQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	r.sql, r.args = sql, args
	return emptyRows{}, nil
}

func (r *recordingQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	r.sql, r.args = sql, args
	return errorRow{err: pgx.ErrNoRows}
}

// emptyRows is a pgx.Rows with no rows.
type emptyRows struct{}

func (emptyRows) Close()                                       {}
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Scan(dest ...any) error                       { return pgx.ErrNoRows }
func (emptyRows) Values() ([]any, error)                       { return nil, nil }
func (emptyRows) RawValues() [][]byte                          { return nil }
func (emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (emptyRows) Conn() *pgx.Conn                              { return nil }

// errorRow is a pgx.Row whose Scan always returns the given error.
type errorRow struct {
	err error
}

func (e errorRow) Scan(dest ...any) error { return e.err }
func (e errorRow) Err() error             { return e.err }

// TestOptionBeatsCtx verifies that an explicit repo.Owner option takes
// precedence over the user carried in the context.
func TestOptionBeatsCtx(t *testing.T) {
	t.Parallel()

	ctx := user.WithUser(context.Background(), "globex")
	q := &recordingQuerier{}
	r := &pgRepository[entity.Asset]{scope: &noopScope{q: q}, table: "assets", scanRow: scanAsset}

	_, err := r.Get(ctx, "some-id", repo.Owner("acme"))
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound from fake row", err)
	}

	found := false
	for _, a := range q.args {
		if a == "acme" {
			found = true
		}
		if a == "globex" {
			t.Fatalf("owner_id arg = %q, want option %q to beat ctx", a, "acme")
		}
	}
	if !found {
		t.Fatalf("args = %v, want owner_id = %q", q.args, "acme")
	}
	if !strings.Contains(q.sql, "owner_id = $") {
		t.Fatalf("sql = %q, want owner_id scoping", q.sql)
	}
}

// assetRepo builds an asset repository with the given querier and filter config.
func assetRepo(q Querier) *pgRepository[entity.Asset] {
	return &pgRepository[entity.Asset]{
		scope:   &noopScope{q: q},
		table:   "assets",
		scanRow: scanAsset,
		toMap:   assetToMap,
		filters: assetFilters,
	}
}

// TestUnknownFilterField verifies that a filter field outside the entity
// whitelist is rejected with an error before any query is issued.
func TestUnknownFilterField(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		op   string
	}{
		{"Get", "="},
		{"List", "="},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := assetRepo(&panicQuerier{})
			opts := []repo.Option{
				repo.Owner("acme"),
				repo.Where("1; DROP TABLE assets--", tc.op, "x"),
			}
			var err error
			switch tc.name {
			case "Get":
				_, err = r.Get(context.Background(), "some-id", opts...)
			case "List":
				_, err = r.List(context.Background(), opts...)
			}
			if err == nil {
				t.Fatal("err = nil, want unknown filter field error")
			}
			if !strings.Contains(err.Error(), "unknown filter field") {
				t.Fatalf("err = %q, want unknown filter field message", err)
			}
		})
	}
}

// TestUnknownOperator verifies that an operator outside the fixed set is
// rejected with an error before any query is issued.
func TestUnknownOperator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		op     string
		method string
	}{
		{"Get LIKE-ish op", "ILIKE", "Get"},
		{"Get injection op", "= OR 1=1--", "Get"},
		{"List injection op", "AND 1=1--", "List"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := assetRepo(&panicQuerier{})
			opts := []repo.Option{repo.Owner("acme"), repo.Where("brand", tc.op, "x")}
			var err error
			if tc.method == "Get" {
				_, err = r.Get(context.Background(), "some-id", opts...)
			} else {
				_, err = r.List(context.Background(), opts...)
			}
			if err == nil {
				t.Fatal("err = nil, want unknown operator error")
			}
			if !strings.Contains(err.Error(), "unknown operator") {
				t.Fatalf("err = %q, want unknown operator message", err)
			}
		})
	}
}

// TestUnknownOrderColumn verifies that an OrderBy token outside the order
// whitelist is rejected with an error before any query is issued.
func TestUnknownOrderColumn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		orderBy string
	}{
		{"single unknown", "secret"},
		{"second token unknown", "created_at, secret"},
		{"injection shape", "created_at, id; DROP TABLE assets--"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := assetRepo(&panicQuerier{})
			_, err := r.List(context.Background(), repo.Owner("acme"), repo.OrderBy(tc.orderBy))
			if err == nil {
				t.Fatal("err = nil, want unknown order column error")
			}
			if !strings.Contains(err.Error(), "unknown order column") {
				t.Fatalf("err = %q, want unknown order column message", err)
			}
		})
	}
}

// TestInjectionShapeRejected verifies that an injection-shaped filter name is
// rejected with an error before any query is issued (spec scenario:
// "1; DROP TABLE assets--" never reaches SQL).
func TestInjectionShapeRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(r *pgRepository[entity.Asset]) error
	}{
		{"Get", func(r *pgRepository[entity.Asset]) error {
			_, err := r.Get(context.Background(), "some-id",
				repo.Owner("acme"), repo.Where("1; DROP TABLE assets--", "=", "x"))
			return err
		}},
		{"List", func(r *pgRepository[entity.Asset]) error {
			_, err := r.List(context.Background(),
				repo.Owner("acme"), repo.Where("1; DROP TABLE assets--", "=", "x"))
			return err
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := assetRepo(&panicQuerier{})
			err := tc.call(r)
			if err == nil {
				t.Fatal("err = nil, want rejection of injection-shaped name")
			}
			if !strings.Contains(err.Error(), "unknown filter field") {
				t.Fatalf("err = %q, want unknown filter field message", err)
			}
		})
	}
}

// TestIN_NonSlice verifies that the IN operator with a non-slice value is
// rejected with an error before any query is issued.
func TestIN_NonSlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
	}{
		{"string", "a,b"},
		{"int", 42},
		{"nil", nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := assetRepo(&panicQuerier{})
			_, err := r.List(context.Background(),
				repo.Owner("acme"), repo.Where("brand", "IN", tc.value))
			if err == nil {
				t.Fatal("err = nil, want non-slice IN error")
			}
			want := fmt.Sprintf("IN operator requires a slice value, got %T", tc.value)
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %q, want %q", err, want)
			}
		})
	}
}

// TestIN_Slice verifies that the IN operator with a slice value expands to one
// bind placeholder per element and passes each element as a bind argument.
func TestIN_Slice(t *testing.T) {
	t.Parallel()

	q := &recordingQuerier{}
	r := assetRepo(q)

	_, err := r.List(context.Background(),
		repo.Owner("acme"), repo.Where("brand", "IN", []string{"invoice", "warranty"}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.sql, "owner_id = $1") {
		t.Fatalf("sql = %q, want owner_id = $1", q.sql)
	}
	if !strings.Contains(q.sql, "brand IN ($2, $3)") {
		t.Fatalf("sql = %q, want brand IN ($2, $3)", q.sql)
	}
	got := []any{}
	for _, a := range q.args {
		got = append(got, a)
	}
	if len(got) != 3 {
		t.Fatalf("len(args) = %d (%v), want 3", len(got), got)
	}
	if got[1] != "invoice" || got[2] != "warranty" {
		t.Fatalf("args = %v, want invoice, warranty as bind params", got)
	}
	if strings.Contains(q.sql, "invoice") {
		t.Fatalf("sql = %q, must not interpolate slice elements", q.sql)
	}
}

// TestIN_EmptySlice verifies that an empty slice expands to FALSE (IN () is
// invalid SQL) without consuming bind arguments.
func TestIN_EmptySlice(t *testing.T) {
	t.Parallel()

	q := &recordingQuerier{}
	r := assetRepo(q)

	_, err := r.List(context.Background(),
		repo.Owner("acme"), repo.Where("brand", "IN", []string{}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.sql, "FALSE") {
		t.Fatalf("sql = %q, want FALSE condition for empty IN", q.sql)
	}
	if len(q.args) != 1 {
		t.Fatalf("len(args) = %d (%v), want 1 (user only)", len(q.args), q.args)
	}
}

// TestValidFilterAndOrderBy verifies that whitelisted filters and order
// columns produce SQL with whitelisted column names and bind parameters.
func TestValidFilterAndOrderBy(t *testing.T) {
	t.Parallel()

	q := &recordingQuerier{}
	r := assetRepo(q)

	_, err := r.List(context.Background(),
		repo.Owner("acme"),
		repo.Where("norm_brand", "=", "samsung"),
		repo.Where("norm_model", "LIKE", "wf%"),
		repo.OrderBy("created_at, id"),
		repo.Limit(10),
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.sql, "norm_brand = $2") {
		t.Errorf("sql = %q, want norm_brand = $2", q.sql)
	}
	if !strings.Contains(q.sql, "norm_model LIKE $3") {
		t.Errorf("sql = %q, want norm_model LIKE $3", q.sql)
	}
	if !strings.Contains(q.sql, "ORDER BY created_at, id") {
		t.Errorf("sql = %q, want ORDER BY created_at, id", q.sql)
	}
	if !strings.Contains(q.sql, "LIMIT $4") {
		t.Errorf("sql = %q, want LIMIT $4", q.sql)
	}
	got := []any{}
	for _, a := range q.args {
		got = append(got, a)
	}
	want := []any{"acme", "samsung", "wf%", 10}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
