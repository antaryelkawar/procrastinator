package postgres_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"procrastinator-backend/commons/entity"
)

const (
	userA         = "test-user"
	userB         = "test-user-b"
	migrationsDir = "../../migrations"
)

var (
	dsn       string
	migrateMu sync.Mutex
)

func TestMain(m *testing.M) {
	dsn = os.Getenv("PROCRASTINATOR_TEST_DATABASE_URL")
	if dsn == "" {
		os.Setenv("TESTPG_SKIP", "1")
	}
	os.Exit(m.Run())
}

// openTestPool builds an isolated test database: a fresh pool with the given
// schema on the search_path, migrated with goose.
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
	// Drop any stale schema first. 00004 creates fn_visible_households with
	// CREATE OR REPLACE and re-owns it to rls_bypass; a leftover schema from a
	// crashed run holds the function under a different owner and the replace
	// fails with 42501. A clean slate makes every run deterministic.
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("drop stale schema: %w", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	migrateMu.Lock()
	defer migrateMu.Unlock()
	if err := goose.SetDialect("postgres"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	// Isolate goose version tracking to this schema so test packages with
	// different schemas never read each other's migration state.
	goose.SetTableName(schema + ".goose_db_version")
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return pool, nil
}

// ensureUsers idempotently registers the given user IDs in the Users
// table. Every multi-user test that needs additional Users beyond the
// migration-seeded test-user/test-user-b calls this before seeding.
func ensureUsers(ctx context.Context, pool *pgxpool.Pool, ids ...string) error {
	for _, id := range ids {
		if _, err := pool.Exec(ctx, `INSERT INTO Users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id); err != nil {
			return fmt.Errorf("ensureUsers: insert %q: %w", id, err)
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }

func testAsset(overrides ...func(*entity.Asset)) entity.Asset {
	a := entity.Asset{
		OwnerID:      userA,
		Brand:        strPtr("Samsung"),
		Model:        strPtr("WF80A"),
		SerialNumber: strPtr("WM-2024-001"),
		DocType:      entity.DocTypeInvoice,
	}
	for _, fn := range overrides {
		fn(&a)
	}
	return a
}

func testSource() entity.Source {
	return entity.Source{
		Filename:    "INVNAG2302754.pdf",
		ContentType: "application/pdf",
		Size:        48231,
		Path:        "storage/3f2c9a1e-7b4d-4e8a-9c6f-1d2e3f4a5b6c.pdf",
		SHA256:      "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		UploadedAt:  time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC),
	}
}

func assertSourceEqual(t *testing.T, name string, got, want entity.Source) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s: ID = %q, want %q", name, got.ID, want.ID)
	}
	if got.OwnerID != want.OwnerID {
		t.Errorf("%s: OwnerID = %q, want %q", name, got.OwnerID, want.OwnerID)
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

func assertAssetEqual(t *testing.T, name string, got, want entity.Asset) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s: ID = %q, want %q", name, got.ID, want.ID)
	}
	if got.OwnerID != want.OwnerID {
		t.Errorf("%s: OwnerID = %q, want %q", name, got.OwnerID, want.OwnerID)
	}
	assertPtrEqual(t, name+".Brand", got.Brand, want.Brand)
	assertPtrEqual(t, name+".Model", got.Model, want.Model)
	assertPtrEqual(t, name+".SerialNumber", got.SerialNumber, want.SerialNumber)
	assertPtrEqual(t, name+".NormSerial", got.NormSerial, want.NormSerial)
	assertPtrEqual(t, name+".NormBrand", got.NormBrand, want.NormBrand)
	assertPtrEqual(t, name+".NormModel", got.NormModel, want.NormModel)
	assertTimePtrEqual(t, name+".PurchaseDate", got.PurchaseDate, want.PurchaseDate)
	assertTimePtrEqual(t, name+".WarrantyEnd", got.WarrantyEnd, want.WarrantyEnd)
	assertPtrEqual(t, name+".Price", got.Price, want.Price)
	assertPtrEqual(t, name+".Currency", got.Currency, want.Currency)
	if got.DocType != want.DocType {
		t.Errorf("%s: DocType = %q, want %q", name, got.DocType, want.DocType)
	}
	assertMetadataEqual(t, name+".Metadata", got.Metadata, want.Metadata)
}

func assertPtrEqual(t *testing.T, name string, got, want *string) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s: is nil, want %q", name, *want)
	case want == nil:
		t.Errorf("%s: = %q, want nil", name, *got)
	case *got != *want:
		t.Errorf("%s: = %q, want %q", name, *got, *want)
	}
}

func assertTimePtrEqual(t *testing.T, name string, got, want *time.Time) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s: is nil, want %v", name, want)
	case want == nil:
		t.Errorf("%s: = %v, want nil", name, got)
	case !got.Equal(*want):
		t.Errorf("%s: = %v, want %v", name, got, want)
	}
}

func assertMetadataEqual(t *testing.T, name string, got, want map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len = %d, want %d (got %v, want %v)", name, len(got), len(want), got, want)
	}
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("%s: missing key %q", name, k)
			continue
		}
		if g != v {
			t.Errorf("%s: key %q = %v, want %v", name, k, g, v)
		}
	}
}
