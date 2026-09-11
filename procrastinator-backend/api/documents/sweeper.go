package documents

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sweeper periodically resolves expired duplicate-upload pending choices to
// keep_existing (design D7a: when the user does not answer within the window,
// the existing document is kept). It runs cross-user (no owner scope) because
// it is a background janitor, not a request handler.
type Sweeper struct {
	pool     *pgxpool.Pool
	interval time.Duration
	// now is injectable for tests (defaults to time.Now).
	now func() time.Time
}

// NewSweeper constructs a Sweeper bound to the raw pool. The pool is passed
// (rather than the factory) because the sweep query is cross-user and cannot
// go through the owner-scoped repositories.
func NewSweeper(pool *pgxpool.Pool) *Sweeper {
	return &Sweeper{pool: pool, interval: 60 * time.Second, now: time.Now}
}

// SetClock overrides the clock used for expiry checks (test hook).
func (s *Sweeper) SetClock(now func() time.Time) { s.now = now }

// Start runs the sweeper loop until ctx is cancelled. It sweeps once
// immediately on start (restart recovery: resolve already-expired rows that
// accumulated while the process was down), then every interval thereafter.
func (s *Sweeper) Start(ctx context.Context) {
	s.Sweep(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep resolves every non-deleted document whose pending_choice is still
// "pending" and already past its expires_at to keep_existing. It is exported
// so tests can drive a single sweep without the ticker loop.
func (s *Sweeper) Sweep(ctx context.Context) {
	now := s.now()

	// Cross-user query (no owner scope): the sweeper runs outside any request
	// and has no app.user_id, so it cannot use the RLS-backed repositories.
	rows, err := s.pool.Query(ctx, `
		SELECT id, owner_id
		FROM documents
		WHERE deleted_at IS NULL
		  AND (payload #>> '{data,pending_choice,state}') = 'pending'
		  AND ((payload #>> '{data,pending_choice,expires_at}')::timestamptz < $1)`, now)
	if err != nil {
		return
	}
	defer rows.Close()

	type pendingDoc struct {
		id      string
		ownerID string
	}
	var targets []pendingDoc
	for rows.Next() {
		var p pendingDoc
		if err := rows.Scan(&p.id, &p.ownerID); err != nil {
			return
		}
		targets = append(targets, p)
	}
	if err := rows.Err(); err != nil {
		return
	}

	for _, p := range targets {
		if err := s.resolveExpired(ctx, p.id, p.ownerID, now); err != nil {
			// Best-effort janitor: a single row failure (e.g. deleted
			// concurrently) must not abort the whole sweep; the next tick
			// retries.
			continue
		}
	}
}

// resolveExpired rewrites one document's pending_choice to resolved /
// keep_existing. It reads the current payload, mutates the pending_choice
// object, and writes the whole payload back (jsonb has no nested-update
// primitive available through the repositories).
func (s *Sweeper) resolveExpired(ctx context.Context, id, ownerID string, now time.Time) error {
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT payload FROM documents WHERE id = $1`, id).Scan(&payload)
	if err != nil {
		return err
	}

	// Decode {"data": {...}}, update data.pending_choice, re-encode.
	var outer struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &outer); err != nil {
		return err
	}
	if outer.Data == nil {
		return nil // nothing to resolve
	}
	pc, ok := outer.Data["pending_choice"].(map[string]any)
	if !ok {
		return nil
	}
	// Re-check expiry against the (possibly drifted) stored value: only
	// resolve rows that are genuinely past their deadline.
	if expStr, ok := pc["expires_at"].(string); ok {
		if exp, err := time.Parse(time.RFC3339, expStr); err == nil {
			if !exp.Before(now) {
				return nil // not actually expired (clock drift / concurrent write)
			}
		}
	}
	pc["state"] = "resolved"
	pc["outcome"] = "keep_existing"

	newPayload, err := json.Marshal(map[string]any{"data": outer.Data})
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE documents SET payload = $2 WHERE id = $1`, id, newPayload)
	return err
}
