package postgres

import (
	"context"
	"strings"
	"testing"

	"procrastinator-backend/commons/repo"
)

// TestISNULL_List emits owner_household_id IS NULL with no bind arg, so a
// caller can fence on a nullable column (personal-scope fence).
func TestISNULL_List(t *testing.T) {
	t.Parallel()

	q := &visQuerier{firstQuery: true}
	r := visAssetRepo(q)

	_, err := r.List(context.Background(),
		repo.Owner("acme"),
		repo.Where("owner_household_id", "IS NULL", nil),
	)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if !strings.Contains(q.lastSQL, "owner_household_id IS NULL") {
		t.Fatalf("sql = %q, want fragment %q", q.lastSQL, "owner_household_id IS NULL")
	}
	// The IS NULL condition must not add a bind arg: only the owner id arg.
	if len(q.lastArgs) != 1 {
		t.Fatalf("args = %v, want exactly 1 arg (owner id), got %d", q.lastArgs, len(q.lastArgs))
	}
	if q.lastArgs[0] != "acme" {
		t.Fatalf("args = %v, want [acme]", q.lastArgs)
	}
}
