package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Querier abstracts the pgx database operations needed by repositories.
// Both *pgxpool.Pool and pgx.Tx satisfy this interface.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open creates a connection pool from dsn and verifies connectivity.
// The pool is closed if the ping fails.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Migrate applies all pending migrations from migrationsDir using goose.
func (s *Store) Migrate(ctx context.Context, migrationsDir string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: set dialect: %w", err)
	}
	// goose requires *sql.DB; bridge the pool without taking a connection.
	stdDB := stdlib.OpenDBFromPool(s.pool)
	defer stdDB.Close()
	if err := goose.Up(stdDB, migrationsDir); err != nil {
		return fmt.Errorf("postgres: migrate %s: %w", migrationsDir, err)
	}
	return nil
}

// TruncateAll removes all rows from documents, sources, and assets.
// Order respects FK dependencies.
func (s *Store) TruncateAll(ctx context.Context) error {
	const stmt = `TRUNCATE documents, sources, assets CASCADE`
	if _, err := s.pool.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("postgres: truncate all: %w", err)
	}
	return nil
}

// Pool returns the underlying pgxpool.Pool.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Close closes the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}
