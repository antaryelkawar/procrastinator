package entity

import "time"

// BatchStatePreview represents a batch that is validated but not yet committed.
const BatchStatePreview = "preview"

// BatchStateCommitted represents a batch whose lines have been committed to the ledger (terminal).
const BatchStateCommitted = "committed"

// BatchStateDiscarded represents a batch that was discarded without committing (terminal).
const BatchStateDiscarded = "discarded"

// ValidState returns true if s is exactly one of the three batch state constants.
func ValidState(s string) bool {
	return s == BatchStatePreview || s == BatchStateCommitted || s == BatchStateDiscarded
}

// CanTransition reports whether the batch state machine allows moving from from to to.
// The machine is strictly one-way (ST-001/003/004): preview is the only non-terminal state,
// and committed and discarded are terminal — once reached, no further transition is allowed.
// Only preview→committed and preview→discarded are legal; every other transition
// (including self-transitions, committed→discarded, and any transition from an
// unrecognized state) is rejected.
func CanTransition(from, to string) bool {
	return (from == BatchStatePreview && to == BatchStateCommitted) ||
		(from == BatchStatePreview && to == BatchStateDiscarded)
}

// ImportBatch represents one owner-scoped statement import batch (one row in import_batches table).
type ImportBatch struct {
	ID                   string
	OwnerID              string
	State                string
	AccountID            string
	SourceID             string
	Filename             string
	Format               string
	LineCountValid       int
	LineCountDuplicate   int
	LineCountPossibleDup int
	LineCountError       int
	CreatedAt            time.Time
	UpdatedAt            time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
	// DeletedAt is the soft-delete timestamp; nil means the batch is active.
	DeletedAt *time.Time
}

// GetID returns the entity's row identity.
func (b ImportBatch) GetID() string { return b.ID }
