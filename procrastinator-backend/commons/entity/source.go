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
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
	// DeletedAt is the soft-delete timestamp; nil means the source is active.
	DeletedAt *time.Time
}

// GetID returns the entity's row identity.
func (s Source) GetID() string { return s.ID }
