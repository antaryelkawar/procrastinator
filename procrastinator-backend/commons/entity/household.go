package entity

import "time"

// Household represents one row in the households table. ID and OwnerID are
// user-shaped ids (same pattern as users.id).
type Household struct {
	ID          string
	OwnerID     string
	DisplayName string
	CreatedAt   time.Time
}

// HouseholdMember represents one row in household_members: a user↔household membership.
type HouseholdMember struct {
	HouseholdID string
	UserID      string
	CreatedAt   time.Time
}
