package assets

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"procrastinator-backend/internal/store"
)

// testSchema is a private schema owned by this test package. Each DB test
// package gets its own private schema because `go test` runs package test
// binaries concurrently (with default -p) and they share one database — a
// shared public schema would make per-test TRUNCATE isolation race across
// packages.
const testSchema = "lm_assets"

var testDSN string

func TestMain(m *testing.M) {
	testDSN = os.Getenv("LM_TEST_DATABASE_URL")
	if testDSN != "" {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, testDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap pool: %v\n", err)
			os.Exit(1)
		}
		if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE"); err != nil {
			pool.Close()
			fmt.Fprintf(os.Stderr, "drop schema: %v\n", err)
			os.Exit(1)
		}
		if _, err := pool.Exec(ctx, "CREATE SCHEMA "+testSchema); err != nil {
			pool.Close()
			fmt.Fprintf(os.Stderr, "create schema: %v\n", err)
			os.Exit(1)
		}
		pool.Close()
	}
	os.Exit(m.Run())
}

// schemaDSN returns the test DSN with search_path pinned to this package's
// private testSchema, so unqualified DDL/DML lands there.
func schemaDSN() string {
	u, err := url.Parse(testDSN)
	if err != nil {
		return testDSN
	}
	q := u.Query()
	q.Set("search_path", testSchema)
	u.RawQuery = q.Encode()
	return u.String()
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	if testDSN == "" {
		t.Skip("LM_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	st, err := store.Open(context.Background(), schemaDSN())
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	if err := st.Migrate(ctx, migrationsDir()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	if err := st.TruncateAll(ctx); err != nil {
		t.Fatalf("failed to truncate after migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := st.TruncateAll(context.Background()); err != nil {
			t.Logf("cleanup truncate failed: %v", err)
		}
	})
	return st
}

func migrationsDir() string {
	// Package dir is internal/store/assets, so the module-level migrations
	// directory is three levels up.
	return filepath.Join("..", "..", "..", "migrations")
}

func strPtr(s string) *string {
	return &s
}

func datePtr(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func randUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("failed to read random bytes: %v", err)
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// assertAssetFields compares every field of got against want (the original seed).
// It checks raw pointer values for optional fields and non-zero timestamps.
func assertAssetFields(t *testing.T, got, want Asset) {
	t.Helper()

	if got.ID == "" {
		t.Errorf("ID: got empty, want non-empty")
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt: got zero, want non-zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt: got zero, want non-zero")
	}
	if !got.CreatedAt.Before(time.Now().Add(time.Minute)) || !got.CreatedAt.After(time.Now().Add(-time.Minute)) {
		t.Errorf("CreatedAt: %v is not within 1 minute of now", got.CreatedAt)
	}

	cmpStrPtr := func(label string, got, want *string) {
		t.Helper()
		if want == nil && got == nil {
			return
		}
		if want == nil && got != nil {
			t.Errorf("%s: got %q, want nil", label, *got)
			return
		}
		if want != nil && got == nil {
			t.Errorf("%s: got nil, want %q", label, *want)
			return
		}
		if *got != *want {
			t.Errorf("%s: got %q, want %q", label, *got, *want)
		}
	}

	cmpDatePtr := func(label string, got, want *time.Time) {
		t.Helper()
		if want == nil && got == nil {
			return
		}
		if want == nil && got != nil {
			t.Errorf("%s: got %v, want nil", label, *got)
			return
		}
		if want != nil && got == nil {
			t.Errorf("%s: got nil, want %v", label, *want)
			return
		}
		if !got.Equal(*want) {
			t.Errorf("%s: got %v, want %v", label, *got, *want)
		}
	}

	cmpStrPtr("Brand", got.Brand, want.Brand)
	cmpStrPtr("Model", got.Model, want.Model)
	cmpStrPtr("SerialNumber", got.SerialNumber, want.SerialNumber)
	cmpDatePtr("PurchaseDate", got.PurchaseDate, want.PurchaseDate)
	cmpStrPtr("Price", got.Price, want.Price)
	cmpStrPtr("Currency", got.Currency, want.Currency)
	cmpDatePtr("WarrantyStart", got.WarrantyStart, want.WarrantyStart)
	cmpDatePtr("WarrantyEnd", got.WarrantyEnd, want.WarrantyEnd)
}

// All tests run sequentially because they share the same database tables
// and truncate them for isolation. t.Parallel() is intentionally omitted.

func TestCreateReadRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		seed Asset
	}{
		{
			name: "full field set",
			seed: Asset{
				Brand:         strPtr("Apple"),
				Model:         strPtr("MacBook Pro 16"),
				SerialNumber:  strPtr("  SN-123  ABC "),
				PurchaseDate:  datePtr(2025, time.November, 30),
				Price:         strPtr("39999.99"),
				Currency:      strPtr("EUR"),
				WarrantyStart: datePtr(2025, time.December, 1),
				WarrantyEnd:   datePtr(2026, time.November, 30),
			},
		},
		{
			name: "partially null asset",
			seed: Asset{
				SerialNumber: strPtr("SN-9"),
			},
		},
		{
			name: "raw whitespace preserved in raw columns",
			seed: Asset{
				Brand: strPtr("  apple  "),
				Model: strPtr("MacBook  Pro"),
			},
		},
	}

	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := repo.Create(ctx, tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}

			// Verify returned asset from Create matches expectations.
			if got.ID != created.ID {
				t.Errorf("ID mismatch: Create returned %q, GetByID returned %q", created.ID, got.ID)
			}

			// For raw field comparison, use the original seed values.
			assertAssetFields(t, got, tc.seed)
		})
	}
}

func TestPriceRoundTripsExactly(t *testing.T) {
	cases := []struct {
		name  string
		price string
	}{
		{"simple decimal", "39999.99"},
		{"integer", "100"},
		{"high precision 18 digits", "123456789012345.678"},
	}

	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seed := Asset{
				SerialNumber: strPtr(fmt.Sprintf("PRICE-%d", i+1)),
				Price:        strPtr(tc.price),
			}
			created, err := repo.Create(ctx, seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}

			if got.Price == nil {
				t.Fatal("Price: got nil, want non-nil")
			}
			if *got.Price != tc.price {
				t.Errorf("Price: got %q, want %q", *got.Price, tc.price)
			}
		})
	}
}

func TestList(t *testing.T) {
	cases := []struct {
		name  string
		seed  int
		sleep time.Duration
		check func(t *testing.T, repo *Repo, insertedIDs []string)
	}{
		{
			name: "empty table returns empty non-nil slice",
			seed: 0,
			check: func(t *testing.T, repo *Repo, insertedIDs []string) {
				assets, err := repo.List(context.Background())
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(assets) != 0 {
					t.Errorf("len: got %d, want 0", len(assets))
				}
				if assets == nil {
					t.Error("assets is nil, want non-nil empty slice")
				}
			},
		},
		{
			name:  "stable deterministic order",
			seed:  3,
			sleep: 2 * time.Millisecond,
			check: func(t *testing.T, repo *Repo, insertedIDs []string) {
				got1, err := repo.List(context.Background())
				if err != nil {
					t.Fatalf("List (1st): %v", err)
				}
				if len(got1) != len(insertedIDs) {
					t.Fatalf("len: got %d, want %d", len(got1), len(insertedIDs))
				}
				for i, a := range got1 {
					if a.ID != insertedIDs[i] {
						t.Errorf("position %d: got ID %q, want %q", i, a.ID, insertedIDs[i])
					}
				}

				got2, err := repo.List(context.Background())
				if err != nil {
					t.Fatalf("List (2nd): %v", err)
				}
				for i, a := range got2 {
					if a.ID != insertedIDs[i] {
						t.Errorf("stability position %d: got ID %q, want %q", i, a.ID, insertedIDs[i])
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			var ids []string
			for i := 0; i < tc.seed; i++ {
				a, err := repo.Create(ctx, Asset{
					SerialNumber: strPtr(fmt.Sprintf("L%d", i+1)),
				})
				if err != nil {
					t.Fatalf("Create #%d: %v", i+1, err)
				}
				ids = append(ids, a.ID)
				if tc.sleep > 0 && i < tc.seed-1 {
					time.Sleep(tc.sleep)
				}
			}

			tc.check(t, repo, ids)
		})
	}
}

func TestGetByID(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "unknown id returns ErrNotFound",
			id:      "", // placeholder — set from randUUID in test body
			wantErr: true,
		},
		{
			name:    "malformed id returns an error",
			id:      "not-a-uuid",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			id := tc.id
			if id == "" {
				id = randUUID(t)
			}

			_, err := repo.GetByID(ctx, id)
			if tc.name == "unknown id returns ErrNotFound" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			}
		})
	}
}

func TestFindBySerial(t *testing.T) {
	cases := []struct {
		name       string
		seedSerial string
		query      string
		wantErr    bool
	}{
		{
			name:       "finds by exact stored serial",
			seedSerial: "SN-123 ABC",
			query:      "SN-123 ABC",
			wantErr:    false,
		},
		{
			name:       "normalizes case and whitespace in query",
			seedSerial: "  sn-123  abc ",
			query:      "SN-123   ABC",
			wantErr:    false,
		},
		{
			name:       "case-insensitive identity",
			seedSerial: "sn-456",
			query:      "SN-456",
			wantErr:    false,
		},
		{
			name:       "no match returns ErrNotFound",
			seedSerial: "SN-1",
			query:      "SN-2",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			seed, err := repo.Create(ctx, Asset{
				SerialNumber: strPtr(tc.seedSerial),
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repo.FindBySerial(ctx, tc.query)
			if tc.wantErr {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindBySerial: %v", err)
			}
			if got.ID != seed.ID {
				t.Errorf("ID: got %q, want %q", got.ID, seed.ID)
			}
		})
	}
}

func TestFindByBrandModel(t *testing.T) {
	cases := []struct {
		name       string
		seedBrand  *string
		seedModel  *string
		queryBrand string
		queryModel string
		wantErr    bool
	}{
		{
			name:       "finds by exact brand and model",
			seedBrand:  strPtr("Apple"),
			seedModel:  strPtr("MacBook Pro"),
			queryBrand: "Apple",
			queryModel: "MacBook Pro",
			wantErr:    false,
		},
		{
			name:       "normalizes case and whitespace",
			seedBrand:  strPtr("  apple  "),
			seedModel:  strPtr("macbook  pro"),
			queryBrand: "APPLE",
			queryModel: "macbook pro",
			wantErr:    false,
		},
		{
			name:       "brand matches but model differs",
			seedBrand:  strPtr("Apple"),
			seedModel:  strPtr("MacBook Pro"),
			queryBrand: "Apple",
			queryModel: "iMac",
			wantErr:    true,
		},
		{
			name:       "both unknown",
			seedBrand:  strPtr("Apple"),
			seedModel:  strPtr("MacBook Pro"),
			queryBrand: "Dell",
			queryModel: "XPS",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			seed, err := repo.Create(ctx, Asset{
				Brand: tc.seedBrand,
				Model: tc.seedModel,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repo.FindByBrandModel(ctx, tc.queryBrand, tc.queryModel)
			if tc.wantErr {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindByBrandModel: %v", err)
			}
			if got.ID != seed.ID {
				t.Errorf("ID: got %q, want %q", got.ID, seed.ID)
			}
		})
	}
}

func TestCreateOnConflictSerial(t *testing.T) {
	st := openTestStore(t)
	repo := New(st.Pool())
	ctx := context.Background()

	t.Run("concurrent inserts with the same serial converge to one row", func(t *testing.T) {
		type result struct {
			asset   Asset
			created bool
			err     error
		}

		ready := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]result, 2)

		brands := []string{"Race-A", "Race-B"}
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-ready
				a, created, err := repo.CreateOnConflictSerial(ctx, Asset{
					SerialNumber: strPtr("RACE-1"),
					Brand:        strPtr(brands[idx]),
				})
				results[idx] = result{a, created, err}
			}(i)
		}
		close(ready)
		wg.Wait()

		for i, r := range results {
			if r.err != nil {
				t.Fatalf("goroutine %d: %v", i, r.err)
			}
			if r.asset.ID == "" {
				t.Fatalf("goroutine %d: empty ID", i)
			}
		}

		if results[0].asset.ID != results[1].asset.ID {
			t.Errorf("IDs differ: %q vs %q", results[0].asset.ID, results[1].asset.ID)
		}

		var count int
		if err := st.Pool().QueryRow(ctx,
			"SELECT count(*) FROM assets WHERE norm_serial = 'RACE-1'",
		).Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Errorf("count: got %d, want 1", count)
		}

		trueCount := 0
		for _, r := range results {
			if r.created {
				trueCount++
			}
		}
		if trueCount != 1 {
			t.Errorf("created flags: got %d true, want exactly 1", trueCount)
		}
	})

	t.Run("sequential second insert converges to existing row", func(t *testing.T) {
		// RACE-2: the concurrent subtest above already inserted RACE-1 and the
		// table is only truncated once for the whole test, so use a fresh serial.
		a1, created1, err := repo.CreateOnConflictSerial(ctx, Asset{
			SerialNumber: strPtr("RACE-2"),
			Brand:        strPtr("First"),
		})
		if err != nil {
			t.Fatalf("first CreateOnConflictSerial: %v", err)
		}
		if !created1 {
			t.Fatal("first call: expected created=true")
		}

		a2, created2, err := repo.CreateOnConflictSerial(ctx, Asset{
			SerialNumber: strPtr("RACE-2"),
			Brand:        strPtr("Second"),
		})
		if err != nil {
			t.Fatalf("second CreateOnConflictSerial: %v", err)
		}
		if created2 {
			t.Fatal("second call: expected created=false")
		}
		if a2.ID != a1.ID {
			t.Errorf("IDs differ: %q vs %q", a1.ID, a2.ID)
		}

		var count int
		if err := st.Pool().QueryRow(ctx,
			"SELECT count(*) FROM assets WHERE norm_serial = 'RACE-2'",
		).Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Errorf("count: got %d, want 1", count)
		}
	})
}

func TestUpdateFields(t *testing.T) {
	cases := []struct {
		name   string
		seed   Asset
		update UpdateFields
		check  func(t *testing.T, repo *Repo, seeded, updated Asset)
	}{
		{
			name: "non-nil fields overwrite, nil fields preserved",
			seed: Asset{
				Brand:        strPtr("Apple"),
				Model:        strPtr("MacBook"),
				SerialNumber: strPtr("SN-1"),
				Price:        strPtr("100"),
			},
			update: UpdateFields{
				Model: strPtr("MacBook Pro"),
				Price: strPtr("200"),
			},
			check: func(t *testing.T, repo *Repo, seeded, updated Asset) {
				ctx := context.Background()

				if updated.Model == nil || *updated.Model != "MacBook Pro" {
					t.Errorf("Model: want %q", "MacBook Pro")
				}
				if updated.Price == nil || *updated.Price != "200" {
					t.Errorf("Price: want %q", "200")
				}
				if updated.Brand == nil || *updated.Brand != "Apple" {
					t.Errorf("Brand: want %q", "Apple")
				}
				if updated.SerialNumber == nil || *updated.SerialNumber != "SN-1" {
					t.Errorf("SerialNumber: want %q", "SN-1")
				}

				found, err := repo.FindByBrandModel(ctx, "apple", "macbook pro")
				if err != nil {
					t.Fatalf("FindByBrandModel: %v", err)
				}
				if found.ID != updated.ID {
					t.Errorf("FindByBrandModel ID: got %q, want %q", found.ID, updated.ID)
				}

				found2, err := repo.FindBySerial(ctx, "SN-1")
				if err != nil {
					t.Fatalf("FindBySerial: %v", err)
				}
				if found2.ID != updated.ID {
					t.Errorf("FindBySerial ID: got %q, want %q", found2.ID, updated.ID)
				}

				if updated.UpdatedAt.Before(seeded.UpdatedAt) {
					t.Errorf("UpdatedAt: %v is before seed UpdatedAt %v", updated.UpdatedAt, seeded.UpdatedAt)
				}
			},
		},
		{
			name: "adding fields never erases existing",
			seed: Asset{
				Brand: strPtr("Apple"),
				Model: strPtr("MacBook"),
				Price: strPtr("100"),
			},
			update: UpdateFields{
				WarrantyStart: datePtr(2025, time.December, 1),
				WarrantyEnd:   datePtr(2026, time.November, 30),
			},
			check: func(t *testing.T, repo *Repo, seeded, updated Asset) {
				if updated.Brand == nil || *updated.Brand != "Apple" {
					t.Errorf("Brand: want %q", "Apple")
				}
				if updated.Model == nil || *updated.Model != "MacBook" {
					t.Errorf("Model: want %q", "MacBook")
				}
				if updated.Price == nil || *updated.Price != "100" {
					t.Errorf("Price: want %q", "100")
				}
				if updated.WarrantyStart == nil || !updated.WarrantyStart.Equal(time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyStart: want 2025-12-01, got %v", updated.WarrantyStart)
				}
				if updated.WarrantyEnd == nil || !updated.WarrantyEnd.Equal(time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyEnd: want 2026-11-30, got %v", updated.WarrantyEnd)
				}
			},
		},
		{
			name: "serial change moves the normalized identity",
			seed: Asset{
				SerialNumber: strPtr("SN-OLD"),
				Brand:        strPtr("Apple"),
			},
			update: UpdateFields{
				SerialNumber: strPtr("SN-NEW"),
			},
			check: func(t *testing.T, repo *Repo, seeded, updated Asset) {
				ctx := context.Background()

				_, err := repo.FindBySerial(ctx, "SN-OLD")
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("FindBySerial(SN-OLD): expected ErrNotFound, got %v", err)
				}

				found, err := repo.FindBySerial(ctx, "sn-new")
				if err != nil {
					t.Fatalf("FindBySerial(sn-new): %v", err)
				}
				if found.ID != updated.ID {
					t.Errorf("FindBySerial(sn-new) ID: got %q, want %q", found.ID, updated.ID)
				}
			},
		},
		{
			name: "unknown id returns ErrNotFound",
			seed: Asset{}, // no seed needed
			update: UpdateFields{
				Brand: strPtr("X"),
			},
			check: nil, // handled specially in the test body
		},
		{
			name: "all-nil update returns current row unchanged",
			seed: Asset{
				Brand: strPtr("Apple"),
				Price: strPtr("100"),
			},
			update: UpdateFields{},
			check: func(t *testing.T, repo *Repo, seeded, updated Asset) {
				if updated.Brand == nil || *updated.Brand != "Apple" {
					t.Errorf("Brand: want %q", "Apple")
				}
				if updated.Price == nil || *updated.Price != "100" {
					t.Errorf("Price: want %q", "100")
				}
				if !updated.UpdatedAt.Equal(seeded.UpdatedAt) {
					t.Errorf("UpdatedAt: got %v, want %v (should not be bumped)", updated.UpdatedAt, seeded.UpdatedAt)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			if tc.name == "unknown id returns ErrNotFound" {
				id := randUUID(t)
				_, err := repo.UpdateFields(ctx, id, tc.update)
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
				return
			}

			seeded, err := repo.Create(ctx, tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			updated, err := repo.UpdateFields(ctx, seeded.ID, tc.update)
			if err != nil {
				t.Fatalf("UpdateFields: %v", err)
			}

			if tc.check != nil {
				tc.check(t, repo, seeded, updated)
			}
		})
	}
}
