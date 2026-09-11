package entity

import "time"

// Household represents one row in the households table. ID and OwnerID are
// user-shaped ids (same pattern as users.id).
type Household struct {
	ID          string
	OwnerID     string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// DeletedAt is the soft-delete timestamp; nil means the household is active.
	DeletedAt *time.Time
}

// GetID returns the entity's row identity.
func (h Household) GetID() string { return h.ID }

// HouseholdMember represents one row in household_members: a user↔household membership.
type HouseholdMember struct {
	HouseholdID string
	UserID      string
	CreatedAt   time.Time
}
