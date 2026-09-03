package entity

import "time"

// Source represents an uploaded document file (one row in sources table).
type Source struct {
	ID          string
	OwnerID     string
	Filename    string
	ContentType string
	Size        int64
	Path        string
	SHA256      string
	UploadedAt  time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
}
