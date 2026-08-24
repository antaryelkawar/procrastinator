package sources

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
const testSchema = "lm_sources"

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
	// Package dir is internal/store/sources, so the module-level migrations
	// directory is three levels up.
	return filepath.Join("..", "..", "..", "migrations")
}

func strPtr(s string) *string {
	return &s
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

// All tests run sequentially because they share the same database tables
// and truncate them for isolation. t.Parallel() is intentionally omitted.

func TestCreateRetainsAllMetadata(t *testing.T) {
	cases := []struct {
		name string
		seed Source
	}{
		{
			name: "pdf invoice",
			seed: Source{
				Filename:    "invoice-2025-11-30.pdf",
				ContentType: "application/pdf",
				ByteSize:    1048576,
				StoragePath: "storage/abc123.pdf",
				SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
		},
		{
			name: "png receipt",
			seed: Source{
				Filename:    "receipt.png",
				ContentType: "image/png",
				ByteSize:    2048,
				StoragePath: "storage/def456.png",
				SHA256:      "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			},
		},
		{
			name: "jpeg photo",
			seed: Source{
				Filename:    "photo.jpg",
				ContentType: "image/jpeg",
				ByteSize:    999999,
				StoragePath: "storage/ghi789.jpg",
				SHA256:      "fcde2b2edba56bf408601fb721fe9b5c33871ee58abff006fee6f30fb2245854",
			},
		},
		{
			name: "zero-byte file",
			seed: Source{
				Filename:    "empty.pdf",
				ContentType: "application/pdf",
				ByteSize:    0,
				StoragePath: "storage/empty.pdf",
				SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			repo := New(st.Pool())
			ctx := context.Background()

			created, err := repo.Create(ctx, tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if created.ID == "" {
				t.Error("ID: got empty, want non-empty")
			}
			if created.UploadedAt.IsZero() {
				t.Error("UploadedAt: got zero, want non-zero")
			}
			now := time.Now()
			if created.UploadedAt.Before(now.Add(-time.Minute)) || created.UploadedAt.After(now.Add(time.Minute)) {
				t.Errorf("UploadedAt: %v is not within 1 minute of now", created.UploadedAt)
			}

			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.ID != created.ID {
				t.Errorf("ID: got %q, want %q", got.ID, created.ID)
			}
			if got.Filename != tc.seed.Filename {
				t.Errorf("Filename: got %q, want %q", got.Filename, tc.seed.Filename)
			}
			if got.ContentType != tc.seed.ContentType {
				t.Errorf("ContentType: got %q, want %q", got.ContentType, tc.seed.ContentType)
			}
			if got.ByteSize != tc.seed.ByteSize {
				t.Errorf("ByteSize: got %d, want %d", got.ByteSize, tc.seed.ByteSize)
			}
			if got.StoragePath != tc.seed.StoragePath {
				t.Errorf("StoragePath: got %q, want %q", got.StoragePath, tc.seed.StoragePath)
			}
			if got.SHA256 != tc.seed.SHA256 {
				t.Errorf("SHA256: got %q, want %q", got.SHA256, tc.seed.SHA256)
			}
			if !got.UploadedAt.Equal(created.UploadedAt) {
				t.Errorf("UploadedAt: got %v, want %v", got.UploadedAt, created.UploadedAt)
			}
		})
	}
}

func TestGetByIDUnknown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
	}{
		{
			name: "random uuid yields ErrNotFound",
			id:   "", // placeholder — set from randUUID in test body
		},
		{
			name: "malformed id yields an error",
			id:   "not-a-uuid",
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
			if tc.name == "random uuid yields ErrNotFound" {
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
