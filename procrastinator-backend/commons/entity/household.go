package entity

import "time"

// ScopePersonal means the row is owned by the user identified by tenant_id.
const ScopePersonal = "personal"

// ScopeHousehold means the row is owned by the household named in owner_household_id.
const ScopeHousehold = "household"

// ValidScopeType returns true if s is exactly one of the two scope type constants.
// The values describe the scope_type column on tenant-owned rows (sources/assets/documents).
func ValidScopeType(s string) bool {
	return s == ScopePersonal || s == ScopeHousehold
}

// Household represents one row in the households table. ID and TenantID are
// user-shaped ids (same pattern as tenants.id).
type Household struct {
	ID          string
	TenantID    string
	DisplayName string
	CreatedAt   time.Time
}

// HouseholdMember represents one row in household_members: a user↔household membership.
type HouseholdMember struct {
	HouseholdID string
	UserID      string
	CreatedAt   time.Time
}
