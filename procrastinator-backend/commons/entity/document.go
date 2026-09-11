package entity

import "time"

// Document represents one extracted document (one row in documents table).
type Document struct {
	ID               string
	OwnerID          string
	SourceID         string
	AssetID          string
	DocType          string
	ExtractedFields  map[string]any
	RawExtraction    string
	CreatedAt        time.Time
	// Confidence is the LLM extraction confidence (0.0–1.0) carried by the
	// document; nil means absent.
	Confidence *float64
	// UserDirective is the free-text note supplied by the user at ingest or
	// reprocess time. It is persisted as documents.payload.data.user_directive
	// and passed into extraction as the prompt parameter.
	UserDirective string
	UpdatedAt     time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
	// DeletedAt is the soft-delete timestamp; nil means the document is active.
	DeletedAt *time.Time
	// PendingChoice is the duplicate-upload pending-choice state stored in
	// documents.payload.data.pending_choice. It is created on duplicate
	// detection (task 3.3) and resolved by the mutators (task 3.4).
	PendingChoice *PendingChoice
}

// PendingChoice is the duplicate-upload pending-choice state stored in
// documents.payload.data.pending_choice. It is created on duplicate detection
// (task 3.3) and resolved by the mutators (task 3.4).
type PendingChoice struct {
	State     string    `json:"state"`     // "pending" | "resolved"
	Outcome   string    `json:"outcome"`   // "" | "reprocess" | "keep_existing"
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GetID returns the entity's row identity.
func (d Document) GetID() string { return d.ID }

// DocumentWithSource is a Document with joined source metadata for the documents list endpoint.
type DocumentWithSource struct {
	Document
	SourceFilename   string
	SourceUploadedAt time.Time
}
