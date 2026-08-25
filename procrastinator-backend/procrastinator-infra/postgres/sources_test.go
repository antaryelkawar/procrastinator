package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-infra/postgres"
)

const testSchemaSources = "p_sources"

var (
	srcOnce    sync.Once
	srcPool    *pgxpool.Pool
	srcRepo    data.SourceRepository
	srcInitErr error
)

// openTestPool builds an isolated test database: a fresh pool with the given
// schema on the search_path, migrated with goose. Mirrors TestMain's setup,
// but parameterized so each file can own its own schema.
func openTestPool(t *testing.T, schema string) (*pgxpool.Pool, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return pool, nil
}

func sourcesRepo(t *testing.T) data.SourceRepository {
	t.Helper()
	srcOnce.Do(func() {
		srcPool, srcInitErr = openTestPool(t, testSchemaSources)
		if srcInitErr != nil {
			return
		}
		srcRepo = postgres.NewSourceRepo(srcPool)
	})
	if srcInitErr != nil {
		t.Fatalf("init sources test pool: %v", srcInitErr)
	}
	return srcRepo
}

// truncateSources clears all test data. Call at the top of each write test.
func truncateSources(t *testing.T) {
	t.Helper()
	sourcesRepo(t) // ensure the pool is initialized
	if _, err := srcPool.Exec(context.Background(), `TRUNCATE documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate sources: %v", err)
	}
}

func testSource() data.Source {
	return data.Source{
		Filename:    "INVNAG2302754.pdf",
		ContentType: "application/pdf",
		Size:        48231,
		Path:        "storage/3f2c9a1e-7b4d-4e8a-9c6f-1d2e3f4a5b6c.pdf",
		SHA256:      "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		UploadedAt:  time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC),
	}
}

func assertSourceEqual(t *testing.T, name string, got, want data.Source) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s: ID = %q, want %q", name, got.ID, want.ID)
	}
	if got.TenantID != want.TenantID {
		t.Errorf("%s: TenantID = %q, want %q", name, got.TenantID, want.TenantID)
	}
	if got.Filename != want.Filename {
		t.Errorf("%s: Filename = %q, want %q", name, got.Filename, want.Filename)
	}
	if got.ContentType != want.ContentType {
		t.Errorf("%s: ContentType = %q, want %q", name, got.ContentType, want.ContentType)
	}
	if got.Size != want.Size {
		t.Errorf("%s: Size = %d, want %d", name, got.Size, want.Size)
	}
	if got.Path != want.Path {
		t.Errorf("%s: Path = %q, want %q", name, got.Path, want.Path)
	}
	if got.SHA256 != want.SHA256 {
		t.Errorf("%s: SHA256 = %q, want %q", name, got.SHA256, want.SHA256)
	}
	if !got.UploadedAt.Equal(want.UploadedAt) {
		t.Errorf("%s: UploadedAt = %v, want %v", name, got.UploadedAt, want.UploadedAt)
	}
}

func TestSourceCreateGetByID(t *testing.T) {
	repo := sourcesRepo(t)
	truncateSources(t)
	ctx := ctxWithTenant(tenantA)

	uploadedAt := time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)
	created, err := repo.Create(ctx, data.Source{
		Filename:    "INVNAG2302754.pdf",
		ContentType: "application/pdf",
		Size:        48231,
		Path:        "storage/3f2c9a1e-7b4d-4e8a-9c6f-1d2e3f4a5b6c.pdf",
		SHA256:      "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		UploadedAt:  uploadedAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if created.TenantID != tenantA {
		t.Errorf("Create TenantID = %q, want %q", created.TenantID, tenantA)
	}
	if !created.UploadedAt.Equal(uploadedAt) {
		t.Errorf("Create UploadedAt = %v, want %v", created.UploadedAt, uploadedAt)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	assertSourceEqual(t, "source", got, created)

	t.Run("zero uploaded_at falls back to now()", func(t *testing.T) {
		zero, err := repo.Create(ctx, data.Source{
			Filename:    "zero.pdf",
			ContentType: "application/pdf",
			Size:        1,
			Path:        "storage/00000000-0000-0000-0000-000000000000.pdf",
			SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		})
		if err != nil {
			t.Fatalf("Create(zero uploaded_at): %v", err)
		}
		if zero.UploadedAt.IsZero() {
			t.Error("Created UploadedAt is zero, want DB default now()")
		}
	})
}

func TestSourceTenantIsolation(t *testing.T) {
	repo := sourcesRepo(t)
	truncateSources(t)

	created, err := repo.Create(ctxWithTenant(tenantA), testSource())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// GetByID under the other tenant must not see the row.
	if _, err := repo.GetByID(ctxWithTenant(tenantB), created.ID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(other tenant): err = %v, want ErrNotFound", err)
	}
}

func TestSourceGetByIDNotFound(t *testing.T) {
	repo := sourcesRepo(t)
	truncateSources(t)
	randomID := "00000000-0000-0000-0000-000000000000"
	if _, err := repo.GetByID(ctxWithTenant(tenantA), randomID); !errors.Is(err, data.ErrNotFound) {
		t.Errorf("GetByID(random): err = %v, want ErrNotFound", err)
	}
}

func TestSourceTenantLessContext(t *testing.T) {
	repo := sourcesRepo(t)
	truncateSources(t)
	bg := context.Background()

	seeded, err := repo.Create(ctxWithTenant(tenantA), testSource())
	if err != nil {
		t.Fatalf("setup Create: %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			_, err := repo.Create(bg, testSource())
			return err
		}},
		{"GetByID", func() error {
			_, err := repo.GetByID(bg, seeded.ID)
			return err
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, data.ErrNoTenant) {
				t.Errorf("%s: err = %v, want ErrNoTenant", tc.name, err)
			}
		})
	}
}
