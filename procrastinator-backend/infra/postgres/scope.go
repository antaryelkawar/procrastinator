package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txScope abstracts the tenant-bound execution context. Every repository
// operation runs inside scope.run, which guarantees app.tenant_id is set
// transaction-locally before the statement executes.
type txScope interface {
	run(ctx context.Context, tid string, fn func(q Querier) error) error
}

// poolScope opens a short transaction per operation:
// BEGIN → set_config → fn(tx) → COMMIT (ROLLBACK on error).
// This is the only spec-compliant option: session-scoped binding is forbidden
// and pool connections are not stable across calls.
type poolScope struct {
	pool *pgxpool.Pool
}

func (s *poolScope) run(ctx context.Context, tid string, fn func(q Querier) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: scope begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op if committed

	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tid); err != nil {
		return fmt.Errorf("postgres: scope bind: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: scope commit: %w", err)
	}
	return nil
}

// txScopeImpl re-applies set_config on an ambient transaction (inside InTx).
// Idempotent within one tx; commit/rollback ownership stays with InTransaction.
type txScopeImpl struct {
	tx pgx.Tx
}

func (s *txScopeImpl) run(ctx context.Context, tid string, fn func(q Querier) error) error {
	if _, err := s.tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tid); err != nil {
		return fmt.Errorf("postgres: scope rebind: %w", err)
	}
	return fn(s.tx)
}
