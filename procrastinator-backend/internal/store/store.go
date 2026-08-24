package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

type Store struct {
	pool *pgxpool.Pool
	dsn  string
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool for dsn %s: %w", dsn, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database at %s: %w", dsn, err)
	}

	return &Store{
		pool: pool,
		dsn:  dsn,
	}, nil
}

func (s *Store) Migrate(ctx context.Context, migrationsDir string) error {
	sqlDB, err := sql.Open("pgx", s.dsn)
	if err != nil {
		return fmt.Errorf("failed to open sql.DB for migrations: %w", err)
	}
	defer sqlDB.Close()

	sqlDB.SetMaxOpenConns(1)

	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		return fmt.Errorf("goose migration failed: %w", err)
	}

	return nil
}

func (s *Store) TruncateAll(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "TRUNCATE documents, assets, sources")
	if err != nil {
		return fmt.Errorf("failed to truncate tables: %w", err)
	}
	return nil
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}
