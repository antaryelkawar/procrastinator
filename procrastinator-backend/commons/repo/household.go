package repo

import (
	"context"

	"procrastinator-backend/commons/entity"
)

// HouseholdRepository is the boundary for the households table and its
// membership join table. It extends the generic CRUD repository with the
// household_members methods (which live in a join table without a tenant_id
// column and are therefore outside the generic engine).
type HouseholdRepository interface {
	Repository[entity.Household]
	AddMember(ctx context.Context, householdID, userID string, opts ...Option) error
	ListMembers(ctx context.Context, householdID string, opts ...Option) ([]entity.HouseholdMember, error)
	HouseholdsForUser(ctx context.Context, userID string, opts ...Option) ([]string, error)
}
