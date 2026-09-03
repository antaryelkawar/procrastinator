package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// HouseholdRepository is the boundary for the households table and its
// membership join table. It extends the generic CRUD repository with the
// household_members methods (which live in a join table without a owner_id
// column and are therefore outside the generic engine).
//
// Households have NO owner_household_id column, so the generic owner-only
// Get/List would hide households the requester is merely a member of. The
// member-aware variants GetHouseholdVisible and ListHouseholdsVisible apply
// the household visibility rule owner_id = me OR id IN my-households instead.
type HouseholdRepository interface {
	Repository[entity.Household]
	AddMember(ctx context.Context, householdID, userID string, opts ...Option) error
	ListMembers(ctx context.Context, householdID string, opts ...Option) ([]entity.HouseholdMember, error)
	HouseholdsForUser(ctx context.Context, userID string, opts ...Option) ([]string, error)
	// GetHouseholdVisible returns a household the requester may see: the
	// requester is either the owner or a member. repo.ErrNotFound otherwise.
	GetHouseholdVisible(ctx context.Context, id string, opts ...Option) (entity.Household, error)
	// ListHouseholdsVisible returns every household the requester owns or is a
	// member of. The result is never nil.
	ListHouseholdsVisible(ctx context.Context, opts ...Option) ([]entity.Household, error)
	// Exists reports whether a household with the given id exists, independent of
	// the caller's membership. The households RLS policy hides rows the caller is
	// not a member of, so a visibility-aware Get cannot distinguish "unknown
	// household" from "not a member"; this probe bypasses RLS to observe the row
	// globally.
	Exists(ctx context.Context, id string, opts ...Option) (bool, error)
}
